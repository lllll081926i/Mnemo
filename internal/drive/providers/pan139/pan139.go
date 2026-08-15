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
	"os"
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
	routeURL = "https://api.mail.10086.cn/h5/short/long/url"
	refreshURL = "https://api.mail.10086.cn/mcloud/accessToken"
	ua       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	RootID   = "pan139_root"
	chunkSize = 5 * 1024 * 1024 // upload chunk
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
			"trashRestore":    true,
		}, nil),
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

// decodeAuthorization parses Basic base64(user:account:token|...|expiration).
func decodeAuthorization(authorization string) (raw, account, tokenPart, splits0 string, expiration int64, err error) {
	authorization = strings.TrimSpace(strings.TrimPrefix(authorization, "Basic "))
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
	fmt.Sscanf(strs[3], "%d", &expiration)
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
	auth := strings.TrimSpace(strings.TrimPrefix(tok.AccessToken, "Basic "))
	var stored struct {
		Authorization     string `json:"authorization"`
		Account           string `json:"account"`
		PersonalCloudHost string `json:"personalCloudHost"`
	}
	_ = json.Unmarshal([]byte(tok.RefreshToken), &stored)
	if auth == "" {
		auth = strings.TrimSpace(strings.TrimPrefix(stored.Authorization, "Basic "))
	}
	if auth == "" {
		return nil, errors.New("139 云盘未登录")
	}
	account := stored.Account
	if dec, _, acc, _, _, err := decodeAuthorization(auth); err == nil {
		account = acc
		_ = dec
		if next, err2 := refreshAuthorization(hc, auth); err2 == nil && next != auth {
			auth = next
			tok.AccessToken = next
		}
	}
	host := strings.TrimSuffix(stored.PersonalCloudHost, "/")
	if host == "" {
		h, err := ensureHost(hc, auth, account)
		if err != nil {
			return nil, err
		}
		host = h
		stored.PersonalCloudHost = host
		stored.Authorization = auth
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
		Success bool `json:"success"`
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
		"Authorization":      "Basic " + authorization,
		"Mcloud-Channel":     "1000101",
		"Mcloud-Client":      "10701",
		"Mcloud-Sign":        fmt.Sprintf("%s,%s,%s", ts, randStr, sign),
		"Mcloud-Version":     "7.14.0",
		"X-Yun-Api-Version":  "v1",
		"X-Yun-App-Channel":  "10000034",
		"X-Yun-Svc-Type":     "1",
		"Origin":             "https://yun.139.com",
		"Referer":            "https://yun.139.com/w/",
		"User-Agent":         ua,
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
	headers["Content-Type"] = "application/json"
	resp, err := hc.Do(ctx, http.MethodPost, url, headers, strings.NewReader(string(bodyStr)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var wrapper struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	if !wrapper.Success {
		return nil, errors.New(wrapper.Message)
	}
	return wrapper.Data, nil
}

// file139 is a raw 139 file entry.
type file139 struct {
	FileID        string `json:"fileId"`
	Name          string `json:"name"`
	CatalogID     string `json:"catalogId"`
	ParentCatalogID string `json:"parentCatalogId"`
	Size          int64  `json:"size"`
	UpdateTime    string `json:"updateTime"`
	CreateTime    string `json:"createTime"`
	ContentType   string `json:"contentType"`
	Path          string `json:"path"`
	Star          int    `json:"star"`
	IsDir         bool   `json:"-"`
}

// listData is the list response data.
type listData struct {
	Items []file139 `json:"dataList"`
	NextPageCursor string `json:"nextPageCursor"`
}

// ListPage lists one page.
func (d *Driver) ListPage(ctx context.Context, c drive.Context, parentID, marker string) ([]model.File, string, error) {
	pageNum := 1
	startNumber := 0
	if marker != "" {
		// marker encodes the next page number and start offset as "page:start"
		if parts := strings.SplitN(marker, ":", 2); len(parts) == 2 {
			if n, err := strconv.Atoi(parts[0]); err == nil {
				pageNum = n
			}
			if n, err := strconv.Atoi(parts[1]); err == nil {
				startNumber = n
			}
		}
	}
	raw, err := d.personalPost(ctx, c, "/file/list", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"catalogID":         toID(parentID),
		"contentCategory":   1,
		"orderBy":           "updateTime",
		"pageNum":           pageNum,
		"pageSize":          100,
		"sortDirection":     "desc",
		"sortType":          1,
		"searchName":        "",
		"startNumber":       startNumber,
	})
	if err != nil {
		return nil, "", err
	}
	var data listData
	_ = json.Unmarshal(raw, &data)
	items := make([]model.File, 0, len(data.Items))
	for _, it := range data.Items {
		items = append(items, mapFile(it, c.DriveID, parentID))
	}
	nextMarker := ""
	if len(data.Items) >= 100 {
		nextMarker = strconv.Itoa(pageNum+1) + ":" + strconv.Itoa(startNumber+len(data.Items))
	}
	return items, nextMarker, nil
}

func accountOf(c drive.Context) string {
	var stored struct {
		Account string `json:"account"`
	}
	_ = json.Unmarshal([]byte(c.Token.RefreshToken), &stored)
	return stored.Account
}

func toID(id string) string {
	if id == "" || id == "root" || id == "/" || id == RootID {
		return "root"
	}
	return id
}

// Detail returns one file.
func (d *Driver) Detail(ctx context.Context, c drive.Context, fileID string) (*file139, error) {
	raw, err := d.personalPost(ctx, c, "/file/getInfo", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileId":            toID(fileID),
	})
	if err != nil {
		return nil, err
	}
	var it file139
	_ = json.Unmarshal(raw, &it)
	return &it, nil
}

// DownloadInfo returns the download URL.
func (d *Driver) DownloadInfo(ctx context.Context, c drive.Context, fileID string) (string, int64, error) {
	raw, err := d.personalPost(ctx, c, "/file/getDownloadUrl", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileId":            toID(fileID),
	})
	if err != nil {
		return "", 0, err
	}
	var res struct {
		URL  string `json:"url"`
		Size int64  `json:"size"`
	}
	_ = json.Unmarshal(raw, &res)
	return res.URL, res.Size, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	raw, err := d.personalPost(ctx, c, "/file/createCatalog", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"parentCatalogId":   toID(parentID),
		"catalogName":       name,
	})
	if err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	var res struct {
		CatalogID string `json:"catalogId"`
	}
	_ = json.Unmarshal(raw, &res)
	return &drive.MkdirResult{FileID: res.CatalogID}, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	_, err := d.personalPost(ctx, c, "/file/rename", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileId":            toID(fileID),
		"newName":           name,
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
	it, err := d.Detail(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	f := mapFile(*it, c.DriveID, it.ParentCatalogID)
	return &f, nil
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	u, size, err := d.DownloadInfo(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID, URL: u, Size: size,
		Headers: map[string]string{"Referer": "https://yun.139.com/"}, DownloadMode: "proxy", Concurrency: 1,
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
	_, err := d.personalPost(ctx, c, "/file/trash", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileIdList":        fileIDs,
	})
	if err != nil {
		return nil, err
	}
	return fileIDs, nil
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	_, err := d.personalPost(ctx, c, "/file/delete", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileIdList":        ids,
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	_, err := d.personalPost(ctx, c, "/file/restore", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileIdList":        fileIDs,
	})
	if err != nil {
		return nil, err
	}
	return fileIDs, nil
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	_, err := d.personalPost(ctx, c, "/file/move", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileIdList":        ids,
		"toCatalogId":       toID(toParentID),
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	_, err := d.personalPost(ctx, c, "/file/copy", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileIdList":        ids,
		"toCatalogId":       toID(toParentID),
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UploadOneFile uploads a single file in chunks.
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, _ := f.Stat()
	size := info.Size()
	buf := make([]byte, chunkSize)
	var offset int64
	var uploadID string
	for offset < size {
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return err
		}
		chunk := buf[:n]
		uploadID, err = d.uploadChunk(ctx, c, ui.Info.Name, toID(ui.Info.ParentFileID), uploadID, offset, size, chunk)
		if err != nil {
			return err
		}
		offset += int64(n)
		if ui != nil {
			ui.Upload.DownSize = offset
			ui.Upload.DownProcess = int(offset * 100 / size)
		}
	}
	return nil
}

func (d *Driver) uploadChunk(ctx context.Context, c drive.Context, name, parentID, uploadID string, offset, total int64, chunk []byte) (string, error) {
	raw, err := d.personalPost(ctx, c, "/file/upload", map[string]any{
		"commonAccountInfo": map[string]any{"account": accountOf(c), "accountType": 1},
		"fileName":          name,
		"parentId":          parentID,
		"fileSize":          total,
		"uploadId":          uploadID,
		"chunkOffset":       offset,
		"chunkSize":         int64(len(chunk)),
		"chunkTotal":        (total + int64(chunkSize) - 1) / int64(chunkSize),
		"chunkData":         base64.StdEncoding.EncodeToString(chunk),
		"sortType":          1,
	})
	if err != nil {
		return "", err
	}
	var res struct {
		UploadID string `json:"uploadId"`
	}
	_ = json.Unmarshal(raw, &res)
	return res.UploadID, nil
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	hc := netx.NewClient(60 * time.Second)
	cr, err := loadCred(hc, token)
	if err != nil {
		return nil, err
	}
	// quota
	raw, err := d.personalPost(ctx, c, "/file/getDiskInfo", map[string]any{
		"commonAccountInfo": map[string]any{"account": cr.account, "accountType": 1},
	})
	if err == nil {
		var disk struct {
			UsedSize  string `json:"usedSize"`
			TotalSize string `json:"totalSize"`
		}
		_ = json.Unmarshal(raw, &disk)
		fmt.Sscanf(disk.UsedSize, "%d", &token.UsedSize)
		fmt.Sscanf(disk.TotalSize, "%d", &token.TotalSize)
	}
	return token, nil
}

// mapFile converts a 139 entry to the unified model.
func mapFile(it file139, driveID, parentID string) model.File {
	isDir := it.ContentType == "folder" || (it.Size == 0 && it.Path == "")
	timeUnix := int64(0)
	if parsed, err := time.Parse("2006-01-02 15:04:05", it.UpdateTime); err == nil {
		timeUnix = parsed.Unix()
	}
	f := driveutil.NewFile(driveID, it.FileID, parentID, it.Name, isDir, it.Size, timeUnix)
	f.Path = it.Path
	if it.Star != 0 {
		f.Starred = true
	}
	return f
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
	_, account, _, _, _, err := decodeAuthorization(strings.TrimPrefix(authorization, "Basic "))
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
		host = ""
	}
	uid := account
	if uid == "" {
		uid = next[:16]
	}
	name := "139 " + uid
	stored := map[string]any{"authorization": next, "account": account, "personalCloudHost": host}
	if u := req.Config["username"]; u != "" {
		stored["username"] = u
		stored["password"] = req.Config["password"]
		stored["account"] = account
	}
	tok := &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  next,
		RefreshToken: mustJSON(stored),
		TokenType:    "Basic",
		UserID:       model.BuildUserID(providerID, uid),
		UserName:     name,
		NickName:     name,
		Name:         name,
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