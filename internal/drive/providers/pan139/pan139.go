// Package pan139 implements the 139 cloud drive provider (AList-sourced
// personal_new API with mcloud signature headers).
package pan139

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	routeURL                   = "https://user-njs.yun.139.com/user/route/qryRoutePolicy"
	refreshURL                 = "https://aas.caiyun.feixin.10086.cn:443/tellin/authTokenRefresh.do"
	mailHostURL                = "https://mail.10086.cn"
	mailLoginURL               = "https://mail.10086.cn/Login/Login.ashx"
	mailSMSURL                 = "https://mail.10086.cn/s"
	mailArtifactURL            = "https://smsrebuild1.mail.10086.cn/setting/s"
	thirdLoginURL              = "https://user-njs.yun.139.com/user/thirdlogin"
	ua                         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	RootID                     = "pan139_root"
	pan139UploadPartSize       = int64(100 * 1024 * 1024)
	pan139LargePartSize        = int64(200 * 1024 * 1024)
	pan139LargeUploadThreshold = int64(30 * 1024 * 1024 * 1024)
	pan139MaxPartsPerRequest   = 100
	pan139SMSStateTTL          = 5 * time.Minute
	pan139SMSMinInterval       = time.Minute
	listPageSize               = 50
	pan139ThirdLoginKey1       = "73634235495062495331515373756c734e7253306c673d3d"
	pan139ThirdLoginKey2       = "7150714477323633586746674c337538"
	pan139SMSPhonePublicKey    = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtu3kxYStVjSCblI8+Fe6C1M4J6cnsGZ12a5Dnz/kg2dwsYbYFsXZJNVzsJjnch8Sy6/WuPKeZCPwjTodI+I2Uqm1B4cmR71mv79GCuWJOFQddO8Qtm8R76xkujUp2ugw3OyuTklv8CQslapDzzoZ2iEAc8jTmsqA6anvWBscaijbCQMmpQj/iOTyu68S+W04gUIImmE62dUf2dpxUcozV5bCXdu16ykf1Ks7M68u6NndO+QbpX++ZyJztc+cNlhumFRHL+rt3o5gDiEaA4C2Z5OyCYCa/MkkZbU6Y5SU7ei1QpVVNWn9pFEyF/NbqgyhwuTwBBr+/bOT4K6KbuYbLwIDAQAB"
)

const providerID = model.ProviderPan139

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"copy":            true,
			"createShare":     true,
			"shareExpiration": true,
			"combinedShare":   true,
			"shareHistory":    true,
			"recycleBin":      true,
			"permanentDelete": true,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"sha256"}, []string{"sha256"})
			c.SetConflictPolicies("rename")
			c.SetShareExpirationOptions(0, 1, 7)
		}),
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "login_mode", Type: "select", Label: "登录方式", Required: true, Options: []drive.LoginOption{{Value: "password", Label: "账号密码"}, {Value: "sms", Label: "短信验证码"}}},
			{Key: "username", Type: "text", Label: "手机号/账号", Required: true},
			{Key: "password", Type: "password", Label: "密码", Required: false},
			{Key: "sms_code", Type: "text", Label: "短信验证码", Required: false},
			{Key: "authorization", Type: "text", Label: "Authorization（可选，粘贴直接登录）", Required: false},
			{Key: "mail_cookies", Type: "text", Label: "Cookie（可选）", Required: false},
		}},
		Auth:    authLogin,
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// ---- crypto helpers ----

func sha1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// aesCBCEncryptBase64Payload: iv||ciphertext base64 (random iv).
func aesCBCEncryptBase64Payload(plaintext, keyHex string) string {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	iv := make([]byte, block.BlockSize())
	_, _ = rand.Read(iv)
	padded := pkcs7Pad([]byte(plaintext), block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	combined := append(iv, out...)
	return base64.StdEncoding.EncodeToString(combined)
}

// aesCBCDecryptFromBase64Payload reverses iv||ciphertext.
func aesCBCDecryptFromBase64Payload(b64, keyHex string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) < 16 {
		return "", errors.New("pan139: ciphertext too short")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := raw[:block.BlockSize()]
	ct := raw[block.BlockSize():]
	if len(ct) == 0 || len(ct)%block.BlockSize() != 0 {
		return "", errors.New("pan139: ciphertext block size invalid")
	}
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	return string(pkcs7Unpad(out, block.BlockSize())), nil
}

// aesECBDecryptHex decrypts ECB hex ciphertext.
func aesECBDecryptHex(hexCipher, keyHex string) (string, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return "", err
	}
	ct, err := hex.DecodeString(hexCipher)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(ct) == 0 || len(ct)%block.BlockSize() != 0 {
		return "", errors.New("pan139: ECB ciphertext block size invalid")
	}
	out := make([]byte, len(ct))
	for i := 0; i < len(ct); i += block.BlockSize() {
		block.Decrypt(out[i:i+block.BlockSize()], ct[i:i+block.BlockSize()])
	}
	return string(pkcs7Unpad(out, block.BlockSize())), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) []byte {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return data
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > blockSize || pad > len(data) {
		return data
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return data
		}
	}
	return data[:len(data)-pad]
}

// sortedJSONStringify renders JSON with sorted keys (AList style).
func sortedJSONStringify(v any) string {
	b, _ := json.Marshal(sortedValue(v))
	return string(b)
}

func sortedValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := make(map[string]any, len(t))
		for _, k := range keys {
			m[k] = sortedValue(t[k])
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = sortedValue(x)
		}
		return out
	default:
		return v
	}
}

// calSign computes the mcloud sign (AList calSign).
func calSign(body, ts, randStr string) string {
	enc := encodeURIComponent139(body)
	chars := strings.Split(enc, "")
	sort.Strings(chars)
	b := base64.StdEncoding.EncodeToString([]byte(strings.Join(chars, "")))
	res := md5hex(b) + md5hex(ts+":"+randStr)
	return strings.ToUpper(md5hex(res))
}

func encodeURIComponent139(s string) string {
	r := url.QueryEscape(s)
	r = strings.ReplaceAll(r, "+", "%20")
	r = strings.ReplaceAll(r, "%21", "!")
	r = strings.ReplaceAll(r, "%27", "'")
	r = strings.ReplaceAll(r, "%28", "(")
	r = strings.ReplaceAll(r, "%29", ")")
	r = strings.ReplaceAll(r, "%2A", "*")
	return r
}

func formatTs() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// normalizeAuthorization removes an optional case-insensitive Basic scheme.
func normalizeAuthorization(authorization string) string {
	authorization = strings.TrimSpace(authorization)
	if len(authorization) > len("Basic") && strings.EqualFold(authorization[:len("Basic")], "Basic") {
		separator := authorization[len("Basic")]
		if separator == ' ' || separator == '\t' {
			return strings.TrimSpace(authorization[len("Basic"):])
		}
	}
	return authorization
}

// decodeAuthorization parses Basic base64(user:account:token|...|expiration).
func decodeAuthorization(authorization string) (raw, account, tokenPart, splits0 string, expiration int64, err error) {
	authorization = normalizeAuthorization(authorization)
	decoded, err := base64.StdEncoding.DecodeString(authorization)
	if err != nil {
		return "", "", "", "", 0, errors.New("pan139: authorization 无效")
	}
	splits := strings.Split(string(decoded), ":")
	if len(splits) < 3 {
		return "", "", "", "", 0, errors.New("pan139: authorization 无效")
	}
	account = splits[1]
	tokenPart = strings.Join(splits[2:], ":")
	strs := strings.Split(tokenPart, "|")
	if len(strs) < 4 {
		return "", "", "", "", 0, errors.New("pan139: authorization token 无效")
	}
	if _, err := fmt.Sscanf(strs[len(strs)-1], "%d", &expiration); err != nil || expiration <= 0 {
		return "", "", "", "", 0, errors.New("pan139: authorization expiration 无效")
	}
	return authorization, account, tokenPart, splits[0], expiration, nil
}

func encodeAuthorization(splits0, account, token string) string {
	return base64.StdEncoding.EncodeToString([]byte(splits0 + ":" + account + ":" + token))
}

// refreshAuthorization refreshes a near-expiry token.
func refreshAuthorization(hc *netx.Client, authorization string) (string, error) {
	_, account, tokenPart, splits0, expiration, err := decodeAuthorization(authorization)
	if err != nil {
		return "", err
	}
	remain := expiration - time.Now().UnixMilli()
	if remain > 15*24*60*60*1000 {
		return authorization, nil
	}
	if remain < 0 {
		return "", errors.New("authorization 已过期，请重新登录")
	}
	reqBody := fmt.Sprintf("<root><token>%s</token><account>%s</account><clienttype>656</clienttype></root>", tokenPart, account)
	resp, err := hc.Do(context.Background(), http.MethodPost, refreshURL, map[string]string{"Content-Type": "application/xml", "User-Agent": ua}, strings.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	ret := regexp.MustCompile(`<return>([^<]*)</return>`).FindStringSubmatch(string(body))
	tok := regexp.MustCompile(`<token>([^<]*)</token>`).FindStringSubmatch(string(body))
	if len(ret) < 2 || ret[1] != "0" || len(tok) < 2 || tok[1] == "" {
		desc := regexp.MustCompile(`<desc>([^<]*)</desc>`).FindStringSubmatch(string(body))
		msg := "刷新 139 token 失败"
		if len(desc) > 1 {
			msg += ": " + desc[1]
		}
		return "", errors.New(msg)
	}
	return encodeAuthorization(splits0, account, tok[1]), nil
}

// cred is the parsed 139 session.
type cred struct {
	authorization string
	account       string
	host          string
}

// loadCred refreshes and returns the session credentials.
func loadCred(hc *netx.Client, tok *model.TokenInfo) (*cred, error) {
	if tok == nil {
		return nil, errors.New("139 云盘未登录")
	}
	auth := normalizeAuthorization(tok.AccessToken)
	var stored struct {
		Authorization     string `json:"authorization"`
		Account           string `json:"account"`
		PersonalCloudHost string `json:"personalCloudHost"`
	}
	_ = json.Unmarshal([]byte(tok.RefreshToken), &stored)
	if auth == "" {
		auth = normalizeAuthorization(stored.Authorization)
	}
	if auth == "" {
		return nil, errors.New("139 云盘未登录")
	}
	account := stored.Account
	authChanged := false
	_, _, acc, _, _, err := decodeAuthorization(auth)
	if err != nil {
		return nil, err
	}
	account = acc
	next, err := refreshAuthorization(hc, auth)
	if err != nil {
		return nil, err
	}
	if next != auth {
		auth = next
		tok.AccessToken = next
		authChanged = true
	}
	host := strings.TrimSuffix(stored.PersonalCloudHost, "/")
	if host == "" {
		h, err := ensureHost(hc, auth, account)
		if err != nil {
			return nil, err
		}
		host = h
		stored.PersonalCloudHost = host
	}
	if authChanged || stored.Authorization != auth || stored.Account != account || stored.PersonalCloudHost != host {
		stored.Authorization = auth
		stored.Account = account
		stored.PersonalCloudHost = host
		tok.RefreshToken = mustJSON(stored)
	}
	return &cred{authorization: auth, account: account, host: host}, nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ensureHost resolves the personal cloud host via the route API.
func ensureHost(hc *netx.Client, authorization, account string) (string, error) {
	body := map[string]any{
		"userInfo":    map[string]any{"userType": 1, "accountType": 1, "accountName": account},
		"modAddrType": 1,
	}
	bodyStr, _ := json.Marshal(body)
	randStr := randomHex(8)
	ts := formatTs()
	sign := calSign(string(bodyStr), ts, randStr)
	var res struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			RoutePolicyList []struct {
				ModName  string `json:"modName"`
				HTTPSURL string `json:"httpsUrl"`
			} `json:"routePolicyList"`
		} `json:"data"`
	}
	err := hc.PostJSON(context.Background(), routeURL, mcloudHeaders(authorization, ts, randStr, sign), body, &res)
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", errors.New(res.Message)
	}
	for _, item := range res.Data.RoutePolicyList {
		if item.ModName == "personal" && item.HTTPSURL != "" {
			return strings.TrimSuffix(item.HTTPSURL, "/"), nil
		}
	}
	return "", errors.New("pan139: personal cloud host 为空")
}

func mcloudHeaders(authorization, ts, randStr, sign string) map[string]string {
	return map[string]string{
		"Accept":                 "application/json, text/plain, */*",
		"Authorization":          "Basic " + authorization,
		"CMS-DEVICE":             "default",
		"Caller":                 "web",
		"Inner-Hcy-Router-Https": "1",
		"Mcloud-Channel":         "1000101",
		"Mcloud-Client":          "10701",
		"Mcloud-Route":           "001",
		"Mcloud-Sign":            fmt.Sprintf("%s,%s,%s", ts, randStr, sign),
		"Mcloud-Version":         "7.14.0",
		"X-Yun-Api-Version":      "v1",
		"X-Yun-App-Channel":      "10000034",
		"X-Yun-Svc-Type":         "1",
		"x-DeviceInfo":           "||9|7.14.0|chrome|120.0.0.0|||windows 10||zh-CN|||",
		"x-huawei-channelSrc":    "10000034",
		"x-inner-ntwk":           "2",
		"x-m4c-caller":           "PC",
		"x-m4c-src":              "10002",
		"x-SvcType":              "1",
		"x-yun-channel-source":   "10000034",
		"x-yun-client-info":      "||9|7.14.0|chrome|120.0.0.0|||windows 10||zh-CN|||dW5kZWZpbmVk||",
		"x-yun-module-type":      "100",
		"x-yun-svc-type":         "1",
		"Origin":                 "https://yun.139.com",
		"Referer":                "https://yun.139.com/w/",
		"User-Agent":             ua,
	}
}

// personalPost signs and posts a JSON body to the personal cloud API.
func (d *Driver) personalPost(ctx context.Context, c drive.Context, pathname string, data any) (json.RawMessage, error) {
	hc := netx.NewClient(60 * time.Second)
	cr, err := loadCred(hc, c.Token)
	if err != nil {
		return nil, err
	}
	return d.personalPostWithCred(ctx, hc, cr, pathname, data)
}

// personalPostWithCred reuses an already refreshed personal-cloud session.
// Account refresh needs this to avoid renewing the same token twice before a
// single low-frequency quota request.
func (d *Driver) personalPostWithCred(ctx context.Context, hc *netx.Client, cr *cred, pathname string, data any) (json.RawMessage, error) {
	if hc == nil || cr == nil {
		return nil, errors.New("pan139: 会话不存在")
	}
	bodyStr, _ := json.Marshal(data)
	randStr := randomHex(8)
	ts := formatTs()
	sign := calSign(string(bodyStr), ts, randStr)
	url := cr.host + "/" + strings.TrimPrefix(pathname, "/")
	headers := mcloudHeaders(cr.authorization, ts, randStr, sign)
	headers["Content-Type"] = "application/json;charset=UTF-8"
	resp, err := hc.Do(ctx, http.MethodPost, url, headers, strings.NewReader(string(bodyStr)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("pan139: API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var wrapper struct {
		Success *bool           `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Success != nil && !*wrapper.Success {
		message := strings.TrimSpace(wrapper.Message)
		if message == "" {
			message = "139 云盘 API 返回失败"
		}
		return nil, errors.New(message)
	}
	return wrapper.Data, nil
}

// file139 is a raw 139 file entry.
type file139 struct {
	FileID          pan139FlexString `json:"fileId"`
	Name            string           `json:"name"`
	CatalogID       pan139FlexString `json:"catalogId"`
	ParentCatalogID pan139FlexString `json:"parentCatalogId"`
	ParentFileID    pan139FlexString `json:"parentFileId"`
	Size            pan139FlexInt64  `json:"size"`
	UpdateTime      string           `json:"updateTime"`
	CreateTime      string           `json:"createTime"`
	UpdatedAt       any              `json:"updatedAt"`
	CreatedAt       any              `json:"createdAt"`
	ContentType     string           `json:"contentType"`
	Type            string           `json:"type"`
	CatalogName     string           `json:"catalogName"`
	ContentHash     string           `json:"contentHash"`
	ContentHashAlg  string           `json:"contentHashAlgorithm"`
	Path            string           `json:"path"`
	Star            int              `json:"star"`
	IsDir           bool             `json:"-"`
}

// listData is the list response data.
type listData struct {
	Items          []file139 `json:"items"`
	LegacyItems    []file139 `json:"dataList"`
	NextPageCursor string    `json:"nextPageCursor"`
}

// pan139FlexString accepts provider ids returned as either JSON strings or numbers.
type pan139FlexString string

func (s *pan139FlexString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*s = pan139FlexString(value)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*s = pan139FlexString(number.String())
	return nil
}

func (s pan139FlexString) String() string { return string(s) }

// pan139FlexInt64 accepts file sizes returned as JSON numbers or strings.
type pan139FlexInt64 int64

func (n *pan139FlexInt64) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*n = 0
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		value, err := strconv.ParseInt(number.String(), 10, 64)
		if err == nil {
			*n = pan139FlexInt64(value)
			return nil
		}
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return err
	}
	*n = pan139FlexInt64(parsed)
	return nil
}

// parsePan139Quota accepts the field spellings used by the personal-cloud
// getDiskInfo response. The endpoint has returned both JSON numbers and
// strings across service revisions.
func parsePan139Quota(raw json.RawMessage) (used, total int64, ok bool) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return 0, 0, false
	}
	if nested, exists := values["data"]; exists {
		var nestedValues map[string]json.RawMessage
		if json.Unmarshal(nested, &nestedValues) == nil {
			values = nestedValues
		}
	}
	used, hasUsed := pan139QuotaInt64(values, "usedSize", "used", "useSize", "used_size")
	total, hasTotal := pan139QuotaInt64(values, "totalSize", "total", "diskSize", "total_size")
	if !hasTotal || total <= 0 {
		return 0, 0, false
	}
	if !hasUsed {
		if free, hasFree := pan139QuotaInt64(values, "freeSize", "free", "availableSize", "available"); hasFree {
			used = total - free
			hasUsed = true
		}
	}
	if !hasUsed {
		return 0, 0, false
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}
	return used, total, true
}

func pan139QuotaInt64(values map[string]json.RawMessage, keys ...string) (int64, bool) {
	for _, key := range keys {
		raw, exists := values[key]
		if !exists || string(raw) == "null" {
			continue
		}
		var value pan139FlexInt64
		if err := json.Unmarshal(raw, &value); err == nil {
			return int64(value), true
		}
	}
	return 0, false
}

func applyPan139Quota(token *model.TokenInfo, used, total int64) {
	if token == nil || total <= 0 {
		return
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}
	token.UsedSize = used
	token.TotalSize = total
	token.FreeSize = total - used
}

type pan139CreateData struct {
	FileID    pan139FlexString `json:"fileId"`
	CatalogID pan139FlexString `json:"catalogId"`
	Name      string           `json:"name"`
}

func (d *Driver) ListPage(ctx context.Context, c drive.Context, parentID, marker string) ([]model.File, string, error) {
	raw, err := d.personalPost(ctx, c, "/file/list", map[string]any{
		"imageThumbnailStyleList": []string{"Small", "Large"},
		"orderBy":                 "updated_at",
		"orderDirection":          "DESC",
		"pageInfo": map[string]any{
			"pageCursor": marker,
			"pageSize":   listPageSize,
		},
		"parentFileId": uploadParentID(parentID),
	})
	if err != nil {
		return nil, "", err
	}
	var data listData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, "", fmt.Errorf("pan139: 文件列表响应无效: %w", err)
	}
	entries := data.Items
	if entries == nil {
		entries = data.LegacyItems
	}
	items := make([]model.File, 0, len(entries))
	for _, it := range entries {
		items = append(items, mapFile(it, c.DriveID, parentID))
	}
	nextMarker := strings.TrimSpace(data.NextPageCursor)
	if nextMarker == marker {
		nextMarker = ""
	}
	return items, nextMarker, nil
}

func accountOf(c drive.Context) string {
	if c.Token == nil {
		return ""
	}
	var stored struct {
		Authorization string `json:"authorization"`
		Account       string `json:"account"`
	}
	_ = json.Unmarshal([]byte(c.Token.RefreshToken), &stored)
	if stored.Account != "" {
		return stored.Account
	}
	for _, authorization := range []string{c.Token.AccessToken, stored.Authorization} {
		if _, account, _, _, _, err := decodeAuthorization(authorization); err == nil && account != "" {
			return account
		}
	}
	return ""
}

func normalizePan139IDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func containsPan139Root(ids []string) bool {
	for _, id := range ids {
		if id == "" || id == "/" || id == "root" || id == RootID || id == "0" {
			return true
		}
	}
	return false
}

// uploadParentID is the root representation used by the newer personal-cloud
// upload API. The legacy list API uses "root", while /file/create expects "/".
func uploadParentID(id string) string {
	if id == "" || id == "root" || id == "/" || id == RootID {
		return "/"
	}
	return id
}

func fileRequestID(id string) string {
	return uploadParentID(id)
}

// Detail returns one file.
func (d *Driver) Detail(ctx context.Context, c drive.Context, fileID string) (*file139, error) {
	raw, err := d.personalPost(ctx, c, "/file/get", map[string]any{"fileId": fileRequestID(fileID)})
	if err != nil {
		return nil, err
	}
	var it file139
	if err := json.Unmarshal(raw, &it); err != nil {
		return nil, fmt.Errorf("pan139: 文件详情响应无效: %w", err)
	}
	if it.FileID.String() == "" && it.CatalogID.String() == "" {
		return nil, errors.New("pan139: 文件详情缺少 fileId")
	}
	return &it, nil
}

// DownloadInfo returns the download URL.
func (d *Driver) DownloadInfo(ctx context.Context, c drive.Context, fileID string) (string, int64, error) {
	raw, err := d.personalPost(ctx, c, "/file/getDownloadUrl", map[string]any{"fileId": fileRequestID(fileID)})
	if err != nil {
		return "", 0, err
	}
	var res struct {
		CDNURL   string          `json:"cdnUrl"`
		URL      string          `json:"url"`
		FileName string          `json:"fileName"`
		Size     pan139FlexInt64 `json:"size"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", 0, fmt.Errorf("pan139: 下载地址响应无效: %w", err)
	}
	if strings.TrimSpace(res.CDNURL) == "" && strings.TrimSpace(res.URL) == "" {
		return "", 0, errors.New("pan139: 下载地址为空")
	}
	if strings.TrimSpace(res.CDNURL) != "" {
		return res.CDNURL, int64(res.Size), nil
	}
	return res.URL, int64(res.Size), nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	raw, err := d.personalPost(ctx, c, "/file/create", map[string]any{
		"parentFileId":   uploadParentID(parentID),
		"name":           name,
		"description":    "",
		"type":           "folder",
		"fileRenameMode": "force_rename",
	})
	if err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	var res pan139CreateData
	if err := json.Unmarshal(raw, &res); err != nil {
		return &drive.MkdirResult{Error: fmt.Sprintf("pan139: 创建文件夹响应无效: %v", err)}, nil
	}
	fileID := res.FileID.String()
	if fileID == "" {
		fileID = res.CatalogID.String()
	}
	if fileID == "" {
		return &drive.MkdirResult{Error: "pan139: 创建文件夹未返回 fileId"}, nil
	}
	return &drive.MkdirResult{FileID: fileID}, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	_, err := d.personalPost(ctx, c, "/file/update", map[string]any{
		"fileId":      fileRequestID(fileID),
		"name":        name,
		"description": "",
	})
	if err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name}, nil
}

// ---- driver ----

// Driver implements drive.Driver for 139.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return RootID }

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	items, _, err := d.ListPage(ctx, c, dirID, "")
	return items, err
}

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, _ *drive.ListOptions) (*drive.DirPage, error) {
	items, next, err := d.ListPage(ctx, c, dirID, marker)
	if err != nil {
		return nil, err
	}
	return &drive.DirPage{Items: items, NextMarker: next}, nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	if fileID == RootID || fileID == "/" || fileID == "root" {
		return model.File{DriveID: c.DriveID, FileID: RootID, Name: "139 云盘", NameSearch: "139", IsDir: true, Icon: "iconfile-folder"}, nil
	}
	return d.GetFile(ctx, c, fileID)
}

func (d *Driver) GetFile(ctx context.Context, c drive.Context, fileID string) (*model.File, error) {
	if fileID == RootID || fileID == "/" || fileID == "root" {
		return &model.File{DriveID: c.DriveID, FileID: RootID, Name: "139 云盘", NameSearch: "139", IsDir: true, Icon: "iconfile-folder"}, nil
	}
	it, err := d.Detail(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	parentID := it.ParentFileID.String()
	if parentID == "" {
		parentID = it.ParentCatalogID.String()
	}
	if parentID == "/" || parentID == "root" {
		parentID = RootID
	}
	f := mapFile(*it, c.DriveID, parentID)
	return &f, nil
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	u, size, err := d.DownloadInfo(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID, URL: u, Size: size,
		Headers: map[string]string{
			"Referer":    "https://yun.139.com/",
			"Origin":     "https://yun.139.com",
			"User-Agent": ua,
		},
		DownloadMode: "proxy", Concurrency: 1,
	}, nil
}

func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 0)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID, FileID: fileID, Size: u.Size, Headers: u.Headers,
		Qualities: []model.VideoQuality{{Quality: "origin", Label: "原画", Value: "origin", URL: u.URL, Headers: u.Headers, ForceProxy: true}},
	}, nil
}

func (d *Driver) MkdirR(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	return d.Mkdir(ctx, c, parentID, name)
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	ids := normalizePan139IDs(fileIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	if containsPan139Root(ids) {
		return nil, errors.New("pan139: 根目录不支持移入回收站")
	}
	_, err := d.personalPost(ctx, c, "/recyclebin/batchTrash", map[string]any{"fileIds": ids})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if id := strings.TrimSpace(r.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if containsPan139Root(ids) {
		return nil, errors.New("pan139: 根目录不支持永久删除")
	}
	_, err := d.personalPost(ctx, c, "/file/batchDelete", map[string]any{"fileIds": ids})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return nil, drive.NotSupported("pan139 recycle restore")
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if id := strings.TrimSpace(r.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if containsPan139Root(ids) {
		return nil, errors.New("pan139: 根目录不支持移动")
	}
	_, err := d.personalPost(ctx, c, "/file/batchMove", map[string]any{
		"fileIds":        ids,
		"toParentFileId": uploadParentID(toParentID),
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if id := strings.TrimSpace(r.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if containsPan139Root(ids) {
		return nil, errors.New("pan139: 根目录不支持复制")
	}
	_, err := d.personalPost(ctx, c, "/file/batchCopy", map[string]any{
		"fileIds":        ids,
		"toParentFileId": uploadParentID(toParentID),
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, errors.New("139 云盘未登录")
	}
	hc := netx.NewClient(60 * time.Second)
	cr, err := loadCred(hc, token)
	if err != nil {
		return nil, err
	}
	// getDiskInfo is a single signed account request. A quota failure must not
	// turn a still-valid 139 session into a login failure or erase its last
	// successful capacity snapshot.
	raw, err := d.personalPostWithCred(ctx, hc, cr, "/file/getDiskInfo", map[string]any{
		"commonAccountInfo": map[string]any{
			"account":     cr.account,
			"accountType": 1,
		},
	})
	if err == nil {
		if used, total, ok := parsePan139Quota(raw); ok {
			applyPan139Quota(token, used, total)
		}
	}
	return token, nil
}

// mapFile converts a 139 entry to the unified model.
func mapFile(it file139, driveID, parentID string) model.File {
	fileID := it.FileID.String()
	if fileID == "" {
		fileID = it.CatalogID.String()
	}
	name := strings.TrimSpace(it.Name)
	if name == "" {
		name = strings.TrimSpace(it.CatalogName)
	}
	isDir := strings.EqualFold(strings.TrimSpace(it.Type), "folder") ||
		strings.EqualFold(strings.TrimSpace(it.ContentType), "folder") ||
		(fileID == it.CatalogID.String() && it.CatalogID.String() != "")
	timeUnix := parsePan139Time(it.UpdateTime, it.UpdatedAt)
	f := driveutil.NewFile(driveID, fileID, parentID, name, isDir, int64(it.Size), timeUnix)
	f.Path = it.Path
	if it.Star != 0 {
		f.Starred = true
	}
	if strings.TrimSpace(it.ContentHash) != "" {
		f.ContentHash = strings.TrimSpace(it.ContentHash)
		f.ContentHashName = strings.ToLower(strings.TrimSpace(it.ContentHashAlg))
		if f.ContentHashName == "" {
			f.ContentHashName = "sha256"
		}
	}
	return f
}

func parsePan139Time(primary string, fallback any) int64 {
	for _, value := range []any{primary, fallback} {
		switch v := value.(type) {
		case string:
			value := strings.TrimSpace(v)
			if value == "" {
				continue
			}
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
				if parsed, err := time.Parse(layout, value); err == nil {
					return parsed.Unix()
				}
			}
			if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
				return pan139UnixValue(millis)
			}
		case float64:
			return pan139UnixValue(int64(v))
		case json.Number:
			if number, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
				return pan139UnixValue(number)
			}
		}
	}
	return 0
}

func pan139UnixValue(value int64) int64 {
	if value > 0 && value < 1_000_000_000_000 {
		return value
	}
	if value > 0 {
		return time.UnixMilli(value).Unix()
	}
	return 0
}
