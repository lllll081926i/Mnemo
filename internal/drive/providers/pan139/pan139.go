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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	ua                         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	RootID                     = "pan139_root"
	pan139UploadPartSize       = int64(100 * 1024 * 1024)
	pan139LargePartSize        = int64(200 * 1024 * 1024)
	pan139LargeUploadThreshold = int64(30 * 1024 * 1024 * 1024)
	pan139MaxPartsPerRequest   = 100
)

const providerID = model.ProviderPan139

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"copy":            true,
			"recycleBin":      true,
			"permanentDelete": true,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"sha256"}, []string{"sha256"})
		}),
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "username", Type: "text", Label: "手机号/邮箱", Required: true},
			{Key: "password", Type: "password", Label: "密码", Required: true},
			{Key: "authorization", Type: "text", Label: "Authorization（可选，粘贴直接登录）", Required: false},
			{Key: "mail_cookies", Type: "text", Label: "mail.10086.cn Cookie（可选，含 RMKEY）", Required: false},
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
	key, _ := hex.DecodeString(keyHex)
	block, _ := aes.NewCipher(key)
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
	key, _ := hex.DecodeString(keyHex)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := raw[:block.BlockSize()]
	ct := raw[block.BlockSize():]
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ct)
	return string(pkcs7Unpad(out, block.BlockSize())), nil
}

// aesECBDecryptHex decrypts ECB hex ciphertext.
func aesECBDecryptHex(hexCipher, keyHex string) (string, error) {
	key, _ := hex.DecodeString(keyHex)
	ct, err := hex.DecodeString(hexCipher)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
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

type pan139CreateData struct {
	FileID    pan139FlexString `json:"fileId"`
	CatalogID pan139FlexString `json:"catalogId"`
	Name      string           `json:"name"`
}

type pan139UploadPart struct {
	ParallelHashCtx struct {
		PartOffset int64 `json:"partOffset"`
	} `json:"parallelHashCtx"`
	PartNumber int   `json:"partNumber"`
	PartSize   int64 `json:"partSize"`
}

type pan139UploadPartURL struct {
	PartNumber int    `json:"partNumber"`
	UploadURL  string `json:"uploadUrl"`
}

type pan139UploadCreateData struct {
	FileID      pan139FlexString      `json:"fileId"`
	FileName    string                `json:"fileName"`
	UploadID    string                `json:"uploadId"`
	PartInfos   []pan139UploadPartURL `json:"partInfos"`
	RapidUpload bool                  `json:"rapidUpload"`
	Exist       bool                  `json:"exist"`
}

type pan139UploadURLsData struct {
	FileID    pan139FlexString      `json:"fileId"`
	UploadID  string                `json:"uploadId"`
	PartInfos []pan139UploadPartURL `json:"partInfos"`
}

type pan139UploadSession struct {
	FileID      string `json:"fileId"`
	UploadID    string `json:"uploadId"`
	ContentHash string `json:"contentHash"`
}

// ListPage lists one page.
func (d *Driver) ListPage(ctx context.Context, c drive.Context, parentID, marker string) ([]model.File, string, error) {
	raw, err := d.personalPost(ctx, c, "/file/list", map[string]any{
		"imageThumbnailStyleList": []string{"Small", "Large"},
		"orderBy":                 "updated_at",
		"orderDirection":          "DESC",
		"pageInfo": map[string]any{
			"pageCursor": marker,
			"pageSize":   100,
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

// pan139UploadParts builds the part description expected by /file/create and
// /file/getUploadUrl. The root API accepts at most 100 part descriptions per
// URL request; the caller fetches additional batches as needed.
func pan139UploadParts(size int64) []pan139UploadPart {
	if size < 0 {
		size = 0
	}
	partSize := pan139UploadPartSize
	if size > pan139LargeUploadThreshold {
		partSize = pan139LargePartSize
	}
	partCount := size / partSize
	if size%partSize != 0 {
		partCount++
	}
	if partCount == 0 {
		partCount = 1
	}
	parts := make([]pan139UploadPart, 0, int(partCount))
	for i := int64(0); i < partCount; i++ {
		offset := i * partSize
		length := size - offset
		if length > partSize {
			length = partSize
		}
		if length < 0 {
			length = 0
		}
		part := pan139UploadPart{PartNumber: int(i + 1), PartSize: length}
		part.ParallelHashCtx.PartOffset = offset
		parts = append(parts, part)
	}
	return parts
}

func pan139ContentType(name string) string {
	contentType := mime.TypeByExtension(strings.ToLower(strings.TrimSpace(filepath.Ext(name))))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func hashPan139File(ctx context.Context, f *os.File, ui *model.UploadingUI) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	info, _ := f.Stat()
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	h := sha256.New()
	buf := make([]byte, 1024*1024)
	var hashed int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(buf)
		if n > 0 {
			if _, writeErr := h.Write(buf[:n]); writeErr != nil {
				return "", writeErr
			}
			hashed += int64(n)
			if ui != nil {
				ui.ReportUploadProgress(hashed, size)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func encodePan139UploadSession(session pan139UploadSession) string {
	b, _ := json.Marshal(session)
	return string(b)
}

func decodePan139UploadSession(raw string) (pan139UploadSession, bool) {
	var session pan139UploadSession
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &session) != nil {
		return pan139UploadSession{}, false
	}
	if strings.TrimSpace(session.FileID) == "" || strings.TrimSpace(session.UploadID) == "" {
		return pan139UploadSession{}, false
	}
	return session, true
}

func mergePan139UploadURLs(urls map[int]string, parts []pan139UploadPartURL) {
	for _, part := range parts {
		if part.PartNumber > 0 && strings.TrimSpace(part.UploadURL) != "" {
			urls[part.PartNumber] = strings.TrimSpace(part.UploadURL)
		}
	}
}

func (d *Driver) getPan139UploadURLs(ctx context.Context, c drive.Context, fileID, uploadID string, parts []pan139UploadPart) (map[int]string, error) {
	urls := make(map[int]string, len(parts))
	for start := 0; start < len(parts); start += pan139MaxPartsPerRequest {
		end := start + pan139MaxPartsPerRequest
		if end > len(parts) {
			end = len(parts)
		}
		var response pan139UploadURLsData
		raw, err := d.personalPost(ctx, c, "/file/getUploadUrl", map[string]any{
			"fileId":    fileID,
			"uploadId":  uploadID,
			"partInfos": parts[start:end],
			"commonAccountInfo": map[string]any{
				"account":     accountOf(c),
				"accountType": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return nil, fmt.Errorf("pan139: 上传地址响应无效: %w", err)
		}
		if response.FileID.String() != "" && response.FileID.String() != fileID {
			return nil, errors.New("pan139: 上传地址返回了错误的 fileId")
		}
		if response.UploadID != "" && response.UploadID != uploadID {
			return nil, errors.New("pan139: 上传地址返回了错误的 uploadId")
		}
		mergePan139UploadURLs(urls, response.PartInfos)
	}
	return urls, nil
}

func putPan139UploadPart(ctx context.Context, hc *netx.Client, f *os.File, part pan139UploadPart, uploadURL string) error {
	body := io.NewSectionReader(f, part.ParallelHashCtx.PartOffset, part.PartSize)
	req, err := hc.Req(ctx, http.MethodPut, uploadURL, body)
	if err != nil {
		return err
	}
	req.ContentLength = part.PartSize
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/")
	resp, err := hc.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("pan139: 分片 %d 上传失败 HTTP %d", part.PartNumber, resp.StatusCode)
	}
	return nil
}

// UploadOneFile uploads a file through 139's SHA-256 precreate protocol.
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil || strings.TrimSpace(ui.Info.LocalFilePath) == "" {
		return errors.New("pan139: 上传文件路径为空")
	}
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	ui.Info.Size = size
	contentHash, err := hashPan139File(ctx, f, ui)
	if err != nil {
		return err
	}
	parts := pan139UploadParts(size)
	sessionKey := drive.UploadSessionKey(c.UserID, c.DriveID, ui.Info.ParentFileID, ui.Info.Name, size)
	savedSessionID, savedParts := drive.LoadUploadSessionState(sessionKey)
	session, resumed := decodePan139UploadSession(savedSessionID)
	if !resumed || !strings.EqualFold(session.ContentHash, contentHash) {
		resumed = false
		session = pan139UploadSession{}
		savedParts = nil
	}

	created := pan139UploadCreateData{}
	if resumed {
		created.FileID = pan139FlexString(session.FileID)
		created.UploadID = session.UploadID
	} else {
		initialParts := parts
		if len(initialParts) > pan139MaxPartsPerRequest {
			initialParts = initialParts[:pan139MaxPartsPerRequest]
		}
		raw, err := d.personalPost(ctx, c, "/file/create", map[string]any{
			"contentHash":          contentHash,
			"contentHashAlgorithm": "SHA256",
			"contentType":          pan139ContentType(ui.Info.Name),
			"fileRenameMode":       "auto_rename",
			"name":                 ui.Info.Name,
			"parallelUpload":       false,
			"parentFileId":         uploadParentID(ui.Info.ParentFileID),
			"partInfos":            initialParts,
			"size":                 size,
			"type":                 "file",
		})
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &created); err != nil {
			return fmt.Errorf("pan139: 上传初始化响应无效: %w", err)
		}
		if created.Exist || created.RapidUpload {
			ui.ReportUploadProgress(size, size)
			drive.ClearUploadSession(sessionKey)
			return nil
		}
		session = pan139UploadSession{
			FileID:      created.FileID.String(),
			UploadID:    created.UploadID,
			ContentHash: contentHash,
		}
		if session.FileID == "" || session.UploadID == "" {
			return errors.New("pan139: 上传初始化未返回 fileId 或 uploadId")
		}
		_ = drive.SaveUploadSessionState(sessionKey, encodePan139UploadSession(session), nil)
	}

	if session.FileID == "" {
		session.FileID = created.FileID.String()
	}
	if session.UploadID == "" {
		session.UploadID = created.UploadID
	}
	if session.FileID == "" || session.UploadID == "" {
		return errors.New("pan139: 上传会话缺少 fileId 或 uploadId")
	}
	uploadedSet := make(map[int]bool, len(savedParts))
	for _, partNumber := range savedParts {
		if partNumber >= 1 && partNumber <= len(parts) {
			uploadedSet[partNumber] = true
		}
	}
	uploaded := int64(0)
	for _, part := range parts {
		if uploadedSet[part.PartNumber] {
			uploaded += part.PartSize
		}
	}
	ui.ReportUploadProgress(uploaded, size)

	urls := make(map[int]string, len(created.PartInfos))
	mergePan139UploadURLs(urls, created.PartInfos)
	pending := make([]pan139UploadPart, 0, len(parts)-len(uploadedSet))
	for _, part := range parts {
		if size > 0 && !uploadedSet[part.PartNumber] && strings.TrimSpace(urls[part.PartNumber]) == "" {
			pending = append(pending, part)
		}
	}
	if len(pending) > 0 {
		moreURLs, err := d.getPan139UploadURLs(ctx, c, session.FileID, session.UploadID, pending)
		if err != nil {
			return err
		}
		for partNumber, uploadURL := range moreURLs {
			urls[partNumber] = uploadURL
		}
	}

	hc := netx.NewClient(10 * time.Minute)
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if uploadedSet[part.PartNumber] {
			continue
		}
		if size > 0 && strings.TrimSpace(urls[part.PartNumber]) == "" {
			return fmt.Errorf("pan139: 第 %d 个分片未返回上传地址", part.PartNumber)
		}
		if part.PartSize > 0 {
			if err := putPan139UploadPart(ctx, hc, f, part, urls[part.PartNumber]); err != nil {
				return err
			}
		}
		uploadedSet[part.PartNumber] = true
		uploaded += part.PartSize
		_ = drive.SaveUploadSessionState(sessionKey, encodePan139UploadSession(session), drive.SortedUniqueParts(uploadedSet))
		ui.ReportUploadProgress(uploaded, size)
	}

	if _, err := d.personalPost(ctx, c, "/file/complete", map[string]any{
		"contentHash":          contentHash,
		"contentHashAlgorithm": "SHA256",
		"fileId":               session.FileID,
		"uploadId":             session.UploadID,
	}); err != nil {
		return err
	}
	drive.ClearUploadSession(sessionKey)
	ui.ReportUploadProgress(size, size)
	return nil
}

// RapidUploadByHash probes/commits 139's SHA-256 precreate path. A miss is
// reported as Reuse=false so the migration engine can fall back to a normal
// download and upload; the provider API owns any temporary precreate session.
func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if !strings.EqualFold(strings.TrimSpace(req.Method), "sha256") {
		return &drive.RapidUploadResult{Reuse: false, Message: "139 云盘仅支持 SHA-256 秒传"}, nil
	}
	hashValue := strings.ToLower(strings.TrimSpace(req.Hash))
	if len(hashValue) != sha256.Size*2 {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 SHA-256 指纹"}, nil
	}
	if _, err := hex.DecodeString(hashValue); err != nil {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 SHA-256 指纹"}, nil
	}
	if req.Size < 0 {
		return nil, errors.New("pan139: 文件大小不能为负数")
	}
	parts := pan139UploadParts(req.Size)
	if len(parts) > pan139MaxPartsPerRequest {
		parts = parts[:pan139MaxPartsPerRequest]
	}
	raw, err := d.personalPost(ctx, c, "/file/create", map[string]any{
		"contentHash":          hashValue,
		"contentHashAlgorithm": "SHA256",
		"contentType":          pan139ContentType(req.FileName),
		"fileRenameMode":       "auto_rename",
		"name":                 req.FileName,
		"parallelUpload":       false,
		"parentFileId":         uploadParentID(req.ParentID),
		"partInfos":            parts,
		"size":                 req.Size,
		"type":                 "file",
	})
	if err != nil {
		return nil, err
	}
	var created pan139UploadCreateData
	if err := json.Unmarshal(raw, &created); err != nil {
		return nil, fmt.Errorf("pan139: 秒传响应无效: %w", err)
	}
	if !created.Exist && !created.RapidUpload {
		if created.FileID.String() != "" && created.UploadID != "" {
			key := drive.UploadSessionKey(c.UserID, c.DriveID, req.ParentID, req.FileName, req.Size)
			_ = drive.SaveUploadSessionState(key, encodePan139UploadSession(pan139UploadSession{
				FileID: created.FileID.String(), UploadID: created.UploadID, ContentHash: hashValue,
			}), nil)
		}
		return &drive.RapidUploadResult{Reuse: false, ParentID: req.ParentID, Message: "未命中秒传"}, nil
	}
	if created.FileID.String() != "" {
		key := drive.UploadSessionKey(c.UserID, c.DriveID, req.ParentID, req.FileName, req.Size)
		drive.ClearUploadSession(key)
	}
	return &drive.RapidUploadResult{
		Reuse:    true,
		FileID:   created.FileID.String(),
		ParentID: req.ParentID,
		Message:  "秒传命中",
	}, nil
}

// ResolveTransferHash reads the SHA-256 content fingerprint exposed by the
// newer /file/get endpoint for cross-drive migration.
func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(method), "sha256") {
		return "", nil
	}
	raw, err := d.personalPost(ctx, c, "/file/get", map[string]any{"fileId": fileRequestID(fileID)})
	if err != nil {
		return "", err
	}
	var detail struct {
		ContentHash          string `json:"contentHash"`
		ContentHashAlgorithm string `json:"contentHashAlgorithm"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return "", fmt.Errorf("pan139: 文件指纹响应无效: %w", err)
	}
	if detail.ContentHashAlgorithm != "" && !strings.EqualFold(detail.ContentHashAlgorithm, "sha256") {
		return "", nil
	}
	hashValue := strings.ToLower(strings.TrimSpace(detail.ContentHash))
	if len(hashValue) != sha256.Size*2 {
		return "", nil
	}
	if _, err := hex.DecodeString(hashValue); err != nil {
		return "", nil
	}
	return hashValue, nil
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, errors.New("139 云盘未登录")
	}
	hc := netx.NewClient(60 * time.Second)
	// The migrated personal-cloud API has no verified quota endpoint. Keep
	// RefreshAccount focused on renewing authorization and the resolved host;
	// do not call the removed legacy /file/getDiskInfo endpoint or silently
	// report stale quota values as fresh data.
	if _, err := loadCred(hc, token); err != nil {
		return nil, err
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

// authLogin handles username/password or authorization login.
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	authorization := strings.TrimSpace(req.Config["authorization"])
	if authorization == "" {
		username := strings.TrimSpace(req.Config["username"])
		password := req.Config["password"]
		if username == "" || password == "" {
			return nil, errors.New("pan139: 请输入账号密码或 Authorization")
		}
		var err error
		authorization, err = loginByPassword(ctx, username, password, req.Config["mail_cookies"])
		if err != nil {
			return nil, err
		}
	}
	_, account, _, _, _, err := decodeAuthorization(authorization)
	if err != nil {
		return nil, err
	}
	hc := netx.NewClient(60 * time.Second)
	next, err := refreshAuthorization(hc, authorization)
	if err != nil {
		return nil, err
	}
	host, err := ensureHost(hc, next, account)
	if err != nil {
		return nil, err
	}
	uid := account
	if uid == "" {
		uid = next
		if len(uid) > 16 {
			uid = uid[:16]
		}
	}
	name := strings.TrimSpace(req.Config["username"])
	if name == "" {
		name = account
	}
	if name == "" {
		name = uid
	}
	stored := map[string]any{"authorization": next, "account": account, "personalCloudHost": host}
	if u := req.Config["username"]; u != "" {
		stored["username"] = u
		stored["password"] = req.Config["password"]
		stored["account"] = account
	}
	tok := &model.TokenInfo{
		TokenFrom:         providerID,
		AccessToken:       next,
		RefreshToken:      mustJSON(stored),
		TokenType:         "Basic",
		UserID:            model.BuildUserID(providerID, uid),
		UserName:          name,
		NickName:          name,
		Name:              name,
		ProviderAccountID: account,
		ProviderRootID:    "/",
	}
	return tok, nil
}

// loginByPassword performs the mail.10086.cn password login flow.
func loginByPassword(ctx context.Context, username, password, mailCookies string) (string, error) {
	// Step 1: password login at mail.10086.cn
	hashedPassword := sha1Hex("fetion.com.cn:" + password)
	loginURL := "https://mail.10086.cn/Login/Login.ashx"
	cookies := parseCookieMap(mailCookies)
	delete(cookies, "JSESSIONID")

	form := url.Values{}
	form.Set("UserName", username)
	form.Set("passOld", "")
	form.Set("auto", "on")
	form.Set("Password", hashedPassword)
	form.Set("webIndexPagePwdLogin", "1")
	form.Set("pwdType", "1")
	form.Set("clientId", "1003")
	form.Set("authType", "2")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://mail.10086.cn")
	req.Header.Set("Referer", "https://mail.10086.cn/default.html")
	req.Header.Set("User-Agent", ua)
	if c := formatCookieMap(cookies); c != "" {
		req.Header.Set("Cookie", c)
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	for _, sc := range resp.Header.Values("Set-Cookie") {
		if m := regexp.MustCompile(`RMKEY=([^;]+)`).FindStringSubmatch(sc); len(m) > 1 {
			cookies["RMKEY"] = m[1]
		}
		if m := regexp.MustCompile(`Os_SSo_Sid=([^;]+)`).FindStringSubmatch(sc); len(m) > 1 {
			cookies["Os_SSo_Sid"] = m[1]
		}
	}
	sid := ""
	if m := regexp.MustCompile(`[?&]sid=([^&]+)`).FindStringSubmatch(location); len(m) > 1 {
		sid = m[1]
	}
	if sid == "" {
		if m := regexp.MustCompile(`Os_SSo_Sid=([^;]+)`).FindStringSubmatch(resp.Header.Get("Set-Cookie")); len(m) > 1 {
			sid = m[1]
		}
	}
	if sid == "" {
		return "", errors.New("账密登录失败：未拿到 sid（请检查账号密码，或补充 mail.10086.cn Cookie）")
	}
	rmkey := cookies["RMKEY"]
	if rmkey == "" {
		return "", errors.New("缺少 RMKEY（请从 mail.10086.cn 登录后复制 Cookie）")
	}

	// Step 2: getArtifact
	artifact, err := getArtifact(ctx, sid, rmkey)
	if err != nil {
		return "", err
	}
	// Step 3: authorize with artifact → authorization
	return authorizeWithArtifact(ctx, sid, artifact, cookies, location)
}

func getArtifact(ctx context.Context, sid, rmkey string) (string, error) {
	urlValue := fmt.Sprintf("https://smsrebuild1.mail.10086.cn/setting/s?func=%s&sid=%s&cguid=%s",
		url.QueryEscape("umc:getArtifact"), url.QueryEscape(sid), fmt.Sprint(time.Now().UnixMilli()))
	hc := netx.NewClient(60 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlValue, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Host", "smsrebuild1.mail.10086.cn")
	req.Header.Set("Cookie", "RMKEY="+rmkey)
	req.Header.Set("User-Agent", ua)
	resp, err := hc.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	// artifact is embedded in the response script; parse "getArtifact" value
	m := regexp.MustCompile(`(?s)artifact["']?\s*[:=]\s*["']([^"']+)`).FindStringSubmatch(text)
	if len(m) < 2 {
		m2 := regexp.MustCompile(`(?s)"data"\s*:\s*\{[^}]*"value"\s*:\s*"([^"]+)"`).FindStringSubmatch(text)
		if len(m2) < 2 {
			return "", errors.New("获取 artifact 失败")
		}
		return m2[1], nil
	}
	return m[1], nil
}

// authorizeWithArtifact calls the authorize endpoint to get the Basic authorization.
func authorizeWithArtifact(ctx context.Context, sid, artifact string, cookies map[string]string, referer string) (string, error) {
	cguid := fmt.Sprint(time.Now().UnixMilli())
	urlValue := fmt.Sprintf("https://mail.10086.cn/Login/Authorize.ashx?sid=%s&func=%s&cguid=%s",
		url.QueryEscape(sid), url.QueryEscape("umc:authorize"), cguid)
	form := url.Values{}
	form.Set("func", "umc:authorize")
	form.Set("sid", sid)
	form.Set("cguid", cguid)
	form.Set("appId", "33000002")
	form.Set("redirect_uri", "https://yun.139.com/w/")
	form.Set("action", "http://yun.139.com/w/")
	form.Set("userType", "1")
	form.Set("artifact", artifact)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlValue, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://mail.10086.cn")
	req.Header.Set("Referer", "https://mail.10086.cn/")
	req.Header.Set("User-Agent", ua)
	if c := formatCookieMap(cookies); c != "" {
		req.Header.Set("Cookie", c)
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	m := regexp.MustCompile(`(?s)"authorization"\s*:\s*"([^"]+)"`).FindStringSubmatch(text)
	if len(m) < 2 {
		return "", errors.New("获取 authorization 失败：" + truncate(text, 120))
	}
	return m[1], nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func parseCookieMap(raw string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		i := strings.Index(part, "=")
		if i <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:i])
		value := strings.TrimSpace(part[i+1:])
		if name != "" {
			m[name] = value
		}
	}
	return m
}

func formatCookieMap(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}
