// Package pan123 implements the 123 云盘 provider (AList-sourced web API,
// ported from the legacy Electron pan123 client).
package pan123

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	// RootID is the canonical root sentinel (mirrors legacy PAN123_ROOT).
	RootID = "pan123_root"

	apiMain         = "https://yun.123pan.com/b/api"
	apiSignIn       = "https://login.123pan.com/api/user/sign_in"
	apiUserInfo     = apiMain + "/user/info"
	apiFileList     = apiMain + "/file/list/new"
	apiDownloadInfo = apiMain + "/file/download_info"
	apiUploadReq    = apiMain + "/file/upload_request"
	apiUploadDone   = apiMain + "/file/upload_complete"
	apiUploadDoneV2 = apiMain + "/file/upload_complete/v2"
	// 注意：官方端点拼写即 s3_repare_upload_parts_batch（历史沿用）。
	apiS3Prepare  = apiMain + "/file/s3_repare_upload_parts_batch"
	apiS3Auth     = apiMain + "/file/s3_upload_object/auth"
	apiMove       = apiMain + "/file/mod_pid"
	apiRename     = apiMain + "/file/rename"
	apiTrash      = apiMain + "/file/trash"
	apiDelete     = apiMain + "/file/delete"
	apiShare      = apiMain + "/file/share_create" // 实际为 /share/create
	apiShareURL   = "https://yun.123pan.com/b/api/share/create"
	apiFileDetail = apiMain + "/file/info"

	// ua mirrors the legacy pan123 client user agent.
	ua       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
	referer  = "https://yun.123pan.com/"
	platform = "web"
	appVer   = "3"

	maxFilePool = 8000
)

const providerID = model.ProviderPan123

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search":          true,
			"createShare":     true,
			"shareExpiration": true,
			"sharePassword":   true,
			"shareHistory":    true,
			"importShare":     true,
			"trashView":       true,
			"trashRestore":    true,
			"trashPurge":      true,
			"trashClear":      true,
			"copy":            false,
			"permanentDelete": true,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"md5"}, []string{"md5"})
		}),
		Auth: authLogin,
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "username", Type: "text", Label: "手机号/邮箱", Required: true, Hint: "123 云盘账号"},
			{Key: "password", Type: "password", Label: "密码", Required: true},
		}},
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// ---- signPath (AList drivers/123, os=web version=3) ----

// signTable maps decimal digits 0-9 to letters, mirroring the legacy table.
var signTable = []byte("adefghlmyijnopkqrstubcvwsz")

// crc32sum is the unsigned CRC-32 (IEEE) used by the legacy signPath.
func crc32sum(s string) uint32 {
	h := crc32.NewIEEE()
	_, _ = io.WriteString(h, s)
	return h.Sum32()
}

// signPath computes the two query parameters for an API path:
// [timeSign, dataSignValue] where dataSignValue = "ts-random-dataSign".
func signPath(apiPath string) (string, string) {
	random := fmt.Sprintf("%d", rand.Intn(10000001))
	now := time.Now()
	timestamp := fmt.Sprintf("%d", now.Unix())
	// nowStr is the UTC+8 wall-clock yyyyMMddHHmm.
	utc8 := now.Add(8 * time.Hour).UTC()
	nowStr := utc8.Format("200601021504")
	mapped := make([]byte, len(nowStr))
	for i := 0; i < len(nowStr); i++ {
		d := nowStr[i] - '0'
		if d > 9 {
			d = 0
		}
		mapped[i] = signTable[d]
	}
	timeSign := fmt.Sprintf("%d", crc32sum(string(mapped)))
	data := strings.Join([]string{timestamp, random, apiPath, platform, appVer, timeSign}, "|")
	dataSign := fmt.Sprintf("%d", crc32sum(data))
	return timeSign, strings.Join([]string{timestamp, random, dataSign}, "-")
}

// withAPISign appends the sign parameters to a raw API url.
func withAPISign(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	k, v := signPath(u.Path)
	q := u.Query()
	q.Set(k, v)
	u.RawQuery = q.Encode()
	return u.String()
}

// ---- rate limiting (legacy: concurrency 1 + min interval 700ms + error backoff) ----

var (
	rateMu       sync.Mutex
	lastReqAt    time.Time
	backoffUntil time.Time
	errorStreak  int
)

func pan123RateLimit() {
	rateMu.Lock()
	defer rateMu.Unlock()
	waitUntil := lastReqAt.Add(700 * time.Millisecond)
	if backoffUntil.After(waitUntil) {
		waitUntil = backoffUntil
	}
	if d := time.Until(waitUntil); d > 0 {
		time.Sleep(d)
	}
	lastReqAt = time.Now()
}

func pan123RateLimitError() {
	rateMu.Lock()
	errorStreak++
	base := 1000 * time.Millisecond
	d := base << (errorStreak - 1)
	if errorStreak > 5 {
		d = 20 * time.Second
	}
	backoffUntil = time.Now().Add(d)
	rateMu.Unlock()
}

func pan123RateLimitOK() {
	rateMu.Lock()
	errorStreak = 0
	rateMu.Unlock()
}

// ---- http plumbing ----

type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// apiClient issues signed, rate-limited requests with a concrete bearer token.
type apiClient struct {
	hc    *netx.Client
	token string
}

func newAPIClient(token string) *apiClient {
	return &apiClient{hc: netx.NewClient(60 * time.Second), token: token}
}

func pan123Headers(token string) map[string]string {
	return map[string]string{
		"origin":        "https://yun.123pan.com",
		"referer":       referer,
		"authorization": "Bearer " + token,
		"platform":      platform,
		"app-version":   appVer,
		"content-type":  "application/json",
		"user-agent":    ua,
	}
}

func (a *apiClient) do(ctx context.Context, method, rawURL string, body any, query map[string]string) (*apiResp, error) {
	pan123RateLimit()
	finalURL := withAPISign(rawURL)
	if len(query) > 0 {
		u, err := url.Parse(finalURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		finalURL = u.String()
	}
	headers := pan123Headers(a.token)
	var resp *http.Response
	var err error
	if method == http.MethodGet {
		resp, err = a.hc.Do(ctx, method, finalURL, headers, nil)
	} else {
		resp, err = a.hc.Do(ctx, method, finalURL, headers, netx.JSONBody(body))
	}
	if err != nil {
		pan123RateLimitError()
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var out apiResp
	if err := json.Unmarshal(raw, &out); err != nil {
		if resp.StatusCode >= 400 {
			pan123RateLimitError()
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncateBytes(raw, 300))
		}
		pan123RateLimitError()
		return nil, fmt.Errorf("123: 非法响应: %s", truncateBytes(raw, 200))
	}
	pan123RateLimitOK()
	if resp.StatusCode >= 400 && out.Code == 0 {
		out.Code = resp.StatusCode
	}
	return &out, nil
}

// api is the Driver-level entry: token from context, 401 → auto re-login with
// stored credentials, then code != 0 → error.
func (d *Driver) api(ctx context.Context, c drive.Context, method, rawURL string, body any, query map[string]string) (*apiResp, error) {
	if c.Token == nil || c.Token.AccessToken == "" {
		return nil, errors.New("123 云盘未登录")
	}
	a := newAPIClient(c.Token.AccessToken)
	resp, err := a.do(ctx, method, rawURL, body, query)
	if err != nil {
		return nil, err
	}
	if resp.Code == 401 {
		if nt, ok := reloginFromToken(ctx, c.Token); ok {
			c.Token.AccessToken = nt
			a = newAPIClient(nt)
			resp, err = a.do(ctx, method, rawURL, body, query)
			if err != nil {
				return nil, err
			}
		}
	}
	if resp.Code != 0 {
		msg := resp.Message
		if msg == "" {
			msg = fmt.Sprintf("123 云盘请求失败 code=%d", resp.Code)
		}
		return nil, errors.New(msg)
	}
	return resp, nil
}

// reloginFromToken replays the stored username/password on 401.
func reloginFromToken(ctx context.Context, tok *model.TokenInfo) (string, bool) {
	if tok == nil {
		return "", false
	}
	var cred struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(tok.RefreshToken), &cred); err != nil || cred.Username == "" || cred.Password == "" {
		return "", false
	}
	next, err := pan123Login(ctx, cred.Username, cred.Password)
	if err != nil || next == "" {
		return "", false
	}
	return next, true
}

// ---- login (legacy auth.ts: username+password, no RSA/captcha in web API) ----

// pan123LoginBody builds the sign_in body: email → mail/type, else passport/remember.
func pan123LoginBody(username, password string) map[string]any {
	if strings.Contains(username, "@") {
		return map[string]any{"mail": username, "password": password, "type": 2}
	}
	return map[string]any{"passport": username, "password": password, "remember": true}
}

// pan123Login performs POST /api/user/sign_in and returns the token.
func pan123Login(ctx context.Context, username, password string) (string, error) {
	hc := netx.NewClient(60 * time.Second)
	headers := map[string]string{
		"origin":       "https://yun.123pan.com",
		"referer":      referer,
		"platform":     platform,
		"app-version":  appVer,
		"content-type": "application/json",
		"user-agent":   ua,
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := hc.PostJSON(ctx, apiSignIn, headers, pan123LoginBody(username, password), &out); err != nil {
		return "", err
	}
	if out.Code != 200 {
		msg := out.Message
		if msg == "" {
			msg = "123 云盘登录失败"
		}
		return "", errors.New(msg)
	}
	if out.Data.Token == "" {
		return "", errors.New("123 云盘未返回 token")
	}
	return out.Data.Token, nil
}

// authLogin is the registration AuthFunc: login → user info → TokenInfo.
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	username := strings.TrimSpace(req.Config["username"])
	password := req.Config["password"]
	if username == "" || password == "" {
		return nil, errors.New("请输入 123 云盘账号和密码")
	}
	token, err := pan123Login(ctx, username, password)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if resp, err := newAPIClient(token).do(ctx, http.MethodGet, apiUserInfo, nil, nil); err == nil {
		data = parseMap(resp.Data)
	}
	uid := firstString(data, "Uid", "uid")
	if uid == "" {
		uid = username
	}
	name := firstString(data, "Nickname", "nickname", "Passport", "passport", "Mail", "mail")
	if name == "" {
		name = username
	}
	used := firstInt64(data, "SpaceUsed", "spaceUsed")
	total := firstInt64(data, "SpacePermanent", "spacePermanent") + firstInt64(data, "SpaceTemp", "spaceTemp")
	free := total - used
	if free < 0 {
		free = 0
	}
	tok := &model.TokenInfo{
		TokenFrom:         providerID,
		AccessToken:       token,
		RefreshToken:      mustJSON(map[string]string{"username": username, "password": password}),
		TokenType:         "Bearer",
		UserID:            model.BuildUserID(providerID, uid),
		UserName:          name,
		NickName:          name,
		Name:              name,
		Avatar:            firstString(data, "HeadImage", "headImage"),
		DefaultDriveID:    model.BuildDriveID(providerID, uid),
		ProviderAccountID: uid,
		ProviderRootID:    "0",
		UsedSize:          used,
		TotalSize:         total,
		FreeSize:          free,
	}
	return tok, nil
}

// RefreshAccount refreshes quota + profile from /user/info.
func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, errors.New("123 云盘未登录")
	}
	resp, err := d.api(ctx, c, http.MethodGet, apiUserInfo, nil, nil)
	if err != nil {
		return token, nil
	}
	data := parseMap(resp.Data)
	used := firstInt64(data, "SpaceUsed", "spaceUsed")
	total := firstInt64(data, "SpacePermanent", "spacePermanent") + firstInt64(data, "SpaceTemp", "spaceTemp")
	if used != 0 || total != 0 {
		token.UsedSize = used
		token.TotalSize = total
		token.FreeSize = total - used
		if token.FreeSize < 0 {
			token.FreeSize = 0
		}
	}
	name := firstString(data, "Nickname", "nickname", "Passport", "passport", "Mail", "mail")
	if name != "" {
		token.UserName = name
		token.NickName = name
		token.Name = name
	}
	if avatar := firstString(data, "HeadImage", "headImage"); avatar != "" {
		token.Avatar = avatar
	}
	return token, nil
}

// ---- file pool (AList drivers/123 runtime File pool) ----

var (
	poolMu   sync.Mutex
	filePool = map[string]pan123File{}
)

// putPool normalizes and stores a file; non-empty fields win, but empty
// S3KeyFlag/Etag/FileName from a stub /file/info never poison a listed file.
func putPool(f pan123File) pan123File {
	poolMu.Lock()
	defer poolMu.Unlock()
	id := toPan123FileID(f.FileID)
	if id == "" || id == "0" {
		return f
	}
	if ex, ok := filePool[id]; ok {
		f = mergeFile(f, ex)
	}
	delete(filePool, id)
	filePool[id] = f
	for len(filePool) > maxFilePool {
		for k := range filePool {
			delete(filePool, k)
			break
		}
	}
	return f
}

func poolGet(fileID string) (pan123File, bool) {
	poolMu.Lock()
	defer poolMu.Unlock()
	f, ok := filePool[toPan123FileID(fileID)]
	return f, ok
}

// mergeFile mirrors legacy mergePan123File operator semantics:
// || fields (FileId/FileName/Size/Etag/S3KeyFlag/DownloadUrl/UpdateAt) fall
// back on empty/0; ?? fields (Type/ParentFileId/Category/Status/Trashed) keep
// the normalized incoming value (0 stays 0, empty string falls back).
func mergeFile(in, ex pan123File) pan123File {
	if in.FileID == "" {
		in.FileID = ex.FileID
	}
	if in.FileName == "" {
		in.FileName = ex.FileName
	}
	if in.Size == 0 {
		in.Size = ex.Size
	}
	if in.Etag == "" {
		in.Etag = ex.Etag
	}
	if in.S3KeyFlag == "" {
		in.S3KeyFlag = ex.S3KeyFlag
	}
	if in.DownloadURL == "" {
		in.DownloadURL = ex.DownloadURL
	}
	if in.UpdateAt == "" {
		in.UpdateAt = ex.UpdateAt
	}
	if in.ParentFileID == "" {
		in.ParentFileID = ex.ParentFileID
	}
	return in
}

// ---- pan123File normalization ----

// pan123File is the normalized AList 123 file entry (PascalCase + camelCase).
type pan123File struct {
	FileID       string
	FileName     string
	Size         int64
	Type         int // 1 = folder
	Etag         string
	S3KeyFlag    string
	DownloadURL  string
	UpdateAt     string
	ParentFileID string
	Category     int
	Status       int
	Trashed      int
}

// pickS3 searches any raw key matching /s3.?key.?flag/i.
var s3KeyFlagRe = regexp.MustCompile(`(?i)^s3.?key.?flag$`)

func pickS3(raw map[string]any) string {
	for _, k := range []string{"S3KeyFlag", "s3KeyFlag", "s3keyFlag", "S3keyFlag"} {
		if v := raw[k]; v != nil && asString(v) != "" {
			return asString(v)
		}
	}
	for k, v := range raw {
		if s3KeyFlagRe.MatchString(k) && v != nil && asString(v) != "" {
			return asString(v)
		}
	}
	return ""
}

// normalizePan123File mirrors legacy normalizePan123FileMeta.
func normalizePan123File(raw map[string]any) pan123File {
	f := pan123File{
		FileID:       asString(pick(raw, "FileId", "fileId")),
		FileName:     asString(pick(raw, "FileName", "fileName")),
		Size:         asInt64(pick(raw, "Size", "size")),
		Type:         asInt(pick(raw, "Type", "type")),
		Etag:         asString(pick(raw, "Etag", "etag")),
		S3KeyFlag:    asString(pick(raw, "S3KeyFlag", "s3KeyFlag", "s3keyFlag")),
		DownloadURL:  asString(pick(raw, "DownloadUrl", "downloadUrl")),
		UpdateAt:     asString(pick(raw, "UpdateAt", "updateAt")),
		ParentFileID: asString(pick(raw, "ParentFileId", "parentFileId")),
		Category:     asInt(pick(raw, "Category", "category")),
		Status:       asInt(pick(raw, "Status", "status")),
		Trashed:      asInt(pick(raw, "Trashed", "trashed")),
	}
	if f.S3KeyFlag == "" {
		f.S3KeyFlag = pickS3(raw)
	}
	return f
}

// ---- description backup (legacy encodePan123MetaDesc / decodePan123MetaDesc) ----

var pan123MetaRe = regexp.MustCompile(`pan123meta:([A-Za-z0-9_-]+)`)

func encodePan123MetaDesc(f pan123File) string {
	if f.S3KeyFlag == "" && f.Etag == "" {
		return ""
	}
	payload, err := json.Marshal(map[string]any{
		"S3KeyFlag": f.S3KeyFlag,
		"Etag":      f.Etag,
		"Size":      f.Size,
		"Type":      f.Type,
		"FileName":  f.FileName,
		"FileId":    f.FileID,
	})
	if err != nil {
		return ""
	}
	return "pan123meta:" + base64.RawURLEncoding.EncodeToString(payload)
}

func decodePan123MetaDesc(description string) (pan123File, bool) {
	m := pan123MetaRe.FindStringSubmatch(description)
	if len(m) < 2 {
		return pan123File{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(m[1])
	if err != nil {
		return pan123File{}, false
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return pan123File{}, false
	}
	return normalizePan123File(data), true
}

// ---- id helpers ----

// toPan123FileID maps root sentinels to the API's "0".
func toPan123FileID(id string) string {
	v := strings.TrimSpace(id)
	if v == "" || v == RootID || v == "root" || v == "/" {
		return "0"
	}
	return v
}

// toPan123Number converts an id to the numeric form used in request bodies.
func toPan123Number(id string) int64 {
	n, _ := strconv.ParseInt(toPan123FileID(id), 10, 64)
	return n
}

// parentOf maps a unified parent id into the model's root sentinel form.
func parentOf(parentID string) string {
	if parentID == "" || parentID == "0" {
		return RootID
	}
	return parentID
}

// ---- value helpers ----

func pick(raw map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := raw[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func firstString(raw map[string]any, keys ...string) string {
	return asString(pick(raw, keys...))
}

func firstInt64(raw map[string]any, keys ...string) int64 {
	return asInt64(pick(raw, keys...))
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

func asInt64(v any) int64 {
	switch t := v.(type) {
	case json.Number:
		n, _ := t.Int64()
		return n
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	case int:
		return int64(t)
	case int64:
		return t
	}
	return 0
}

func asInt(v any) int {
	return int(asInt64(v))
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(t))
		return b
	case json.Number:
		n, _ := t.Int64()
		return n != 0
	case float64:
		return t != 0
	}
	return false
}

// parseMap decodes JSON with UseNumber so big file ids survive as digits.
func parseMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func truncateBytes(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

// ---- file mapping (legacy mapPan123FileToDriveFile) ----

func mapFile(item pan123File, driveID, parentID string) model.File {
	isDir := item.Type == 1
	name := item.FileName
	timeUnix := int64(0)
	if item.UpdateAt != "" {
		if t, err := time.Parse(time.RFC3339, item.UpdateAt); err == nil {
			timeUnix = t.Unix()
		} else if t, err := time.Parse("2006-01-02 15:04:05", item.UpdateAt); err == nil {
			timeUnix = t.Unix()
		}
	}
	if timeUnix == 0 {
		timeUnix = time.Now().Unix()
	}
	f := driveutil.NewFile(driveID, item.FileID, parentOf(parentID), name, isDir, item.Size, timeUnix)
	if isDir {
		f.Category = "folder"
		f.Icon = "iconfile-folder"
	}
	f.DownloadURL = item.DownloadURL
	f.Description = encodePan123MetaDesc(item)
	f.ContentHash = ""
	f.ContentHashName = ""
	return f
}

func mapFiles(items []pan123File, driveID, parentID string) []model.File {
	out := make([]model.File, 0, len(items))
	for _, it := range items {
		out = append(out, mapFile(it, driveID, parentID))
	}
	return out
}

// ---- list / search / trash / detail (legacy dirfilelist.ts) ----

// fileListRaw implements the AList getFiles paging loop (guard 200 pages).
func (d *Driver) fileListRaw(ctx context.Context, c drive.Context, parentID string, trashed bool, search string) ([]pan123File, error) {
	parentFileID := toPan123FileID(parentID)
	var res []pan123File
	page := 1
	event := "homeListFile"
	operateType := "4"
	if search != "" {
		event = "homeSearchFile"
		operateType = "2"
	}
	for guard := 0; guard < 200; guard++ {
		query := map[string]string{
			"driveId":              "0",
			"limit":                "100",
			"next":                 "0",
			"orderBy":              "file_id",
			"orderDirection":       "desc",
			"parentFileId":         parentFileID,
			"trashed":              strconv.FormatBool(trashed),
			"SearchData":           search,
			"Page":                 strconv.Itoa(page),
			"OnlyLookAbnormalFile": "0",
			"event":                event,
			"operateType":          operateType,
			"inDirectSpace":        "false",
		}
		resp, err := d.api(ctx, c, http.MethodGet, apiFileList, nil, query)
		if err != nil {
			return nil, err
		}
		data := parseMap(resp.Data)
		list := rawList(data)
		for _, raw := range list {
			if m, ok := raw.(map[string]any); ok {
				res = append(res, putPool(normalizePan123File(m)))
			}
		}
		page++
		next := asString(pick(data, "Next", "next"))
		if len(list) == 0 || next == "-1" {
			break
		}
	}
	return res, nil
}

// rawList extracts InfoList/infoList from a list data map.
func rawList(data map[string]any) []any {
	if v, ok := data["InfoList"].([]any); ok {
		return v
	}
	if v, ok := data["infoList"].([]any); ok {
		return v
	}
	return nil
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	parent := toPan123FileID(dirID)
	items, err := d.fileListRaw(ctx, c, parent, false, "")
	if err != nil {
		return nil, err
	}
	return mapFiles(items, c.DriveID, parent), nil
}

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, _ string, opts *drive.ListOptions) (*drive.DirPage, error) {
	parent := toPan123FileID(dirID)
	search := ""
	if opts != nil {
		search = opts.Search
	}
	items, err := d.fileListRaw(ctx, c, parent, false, search)
	if err != nil {
		return nil, err
	}
	return &drive.DirPage{Items: mapFiles(items, c.DriveID, parent)}, nil
}

func (d *Driver) ListTrash(ctx context.Context, c drive.Context, _ *drive.ListOptions) ([]model.File, error) {
	items, err := d.fileListRaw(ctx, c, "0", true, "")
	if err != nil {
		return nil, err
	}
	return mapFiles(items, c.DriveID, "trash"), nil
}

func (d *Driver) Search(ctx context.Context, c drive.Context, keyword string) ([]model.File, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []model.File{}, nil
	}
	items, err := d.fileListRaw(ctx, c, "0", false, keyword)
	if err != nil {
		return nil, err
	}
	out := make([]model.File, 0, len(items))
	for _, it := range items {
		out = append(out, mapFile(it, c.DriveID, it.ParentFileID))
	}
	return out, nil
}

// detail fetches /file/info (fallback when the pool lacks a listed file).
func (d *Driver) detail(ctx context.Context, c drive.Context, fileID string) (*pan123File, error) {
	resp, err := d.api(ctx, c, http.MethodGet, apiFileDetail, nil, map[string]string{
		"fileId": toPan123FileID(fileID),
		"event":  "fileInfo",
	})
	if err != nil {
		return nil, err
	}
	m := parseMap(resp.Data)
	if len(m) == 0 {
		return nil, errors.New("123: 文件不存在")
	}
	f := putPool(normalizePan123File(m))
	return &f, nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	if fileID == RootID || fileID == "0" {
		return model.File{
			DriveID: c.DriveID, FileID: RootID, ParentFileID: "",
			Name: "123 云盘", NameSearch: "123", IsDir: true, Icon: "iconfile-folder",
		}, nil
	}
	fid := toPan123FileID(fileID)
	if pooled, ok := poolGet(fid); ok {
		f := mapFile(pooled, c.DriveID, pooled.ParentFileID)
		return f, nil
	}
	detail, err := d.detail(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	f := mapFile(*detail, c.DriveID, detail.ParentFileID)
	return f, nil
}

func (d *Driver) GetFile(ctx context.Context, c drive.Context, fileID string) (*model.File, error) {
	v, err := d.GetInfo(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	f, ok := v.(model.File)
	if !ok || f.FileID == "" {
		return nil, errors.New("123: 文件不存在")
	}
	return &f, nil
}

// ---- download (legacy download.ts: AList Link 逐行移植) ----

// resolveAListFile reproduces the Link prerequisite: a listed file with
// S3KeyFlag, from pool → root list → root search.
func (d *Driver) resolveAListFile(ctx context.Context, c drive.Context, fileID string) (*pan123File, error) {
	fid := toPan123FileID(fileID)
	if f, ok := poolGet(fid); ok && f.S3KeyFlag != "" {
		return &f, nil
	}
	if items, err := d.fileListRaw(ctx, c, "0", false, ""); err == nil {
		for i := range items {
			if items[i].FileID == fid && items[i].S3KeyFlag != "" {
				return &items[i], nil
			}
		}
	}
	if items, err := d.fileListRaw(ctx, c, "0", false, fid); err == nil {
		for i := range items {
			if items[i].FileID == fid && items[i].S3KeyFlag != "" {
				return &items[i], nil
			}
		}
	}
	if f, ok := poolGet(fid); ok && f.S3KeyFlag != "" {
		return &f, nil
	}
	return nil, errors.New("can't convert obj（无列表 File，请先打开所在文件夹）")
}

// extractPan123RedirectURL parses either JSON redirect_url or an href in HTML.
func extractPan123RedirectURL(bodyText, baseURL string) string {
	text := string(bodyText)
	redirect := ""
	var body struct {
		Data struct {
			RedirectURL  string `json:"redirect_url"`
			RedirectURL2 string `json:"redirectUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &body); err == nil {
		redirect = body.Data.RedirectURL
		if redirect == "" {
			redirect = body.Data.RedirectURL2
		}
	}
	if redirect == "" {
		if m := hrefRe.FindStringSubmatch(text); len(m) > 1 {
			redirect = m[1]
		}
	}
	if redirect == "" {
		return ""
	}
	u, err := url.Parse(redirect)
	if err != nil {
		return ""
	}
	if base, err := url.Parse(baseURL); err == nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		return u.String()
	}
	return ""
}

var hrefRe = regexp.MustCompile(`(?i)href\s*=\s*["'](https?:[^"']+)["']`)

// alistLink runs the AList Link body: download_info → params b64 → redirect GET.
func (d *Driver) alistLink(ctx context.Context, c drive.Context, f pan123File) (string, map[string]string, error) {
	fileID, _ := strconv.ParseInt(f.FileID, 10, 64)
	if fileID == 0 {
		fileID = toPan123Number(f.FileID)
	}
	data := map[string]any{
		"driveId":   0,
		"etag":      f.Etag,
		"fileId":    fileID,
		"fileName":  f.FileName,
		"s3keyFlag": f.S3KeyFlag,
		"size":      f.Size,
		"type":      f.Type,
	}
	resp, err := d.api(ctx, c, http.MethodPost, apiDownloadInfo, data, nil)
	if err != nil {
		return "", nil, err
	}
	dm := parseMap(resp.Data)
	downloadURL := asString(pick(dm, "DownloadUrl", "downloadUrl"))
	if downloadURL == "" {
		return "", nil, errors.New("DownloadUrl 为空")
	}
	// base64(params) is the real URL when present.
	if u, err := url.Parse(downloadURL); err == nil {
		if nu := u.Query().Get("params"); nu != "" {
			if du, err := base64.StdEncoding.DecodeString(nu); err == nil && len(du) > 0 {
				downloadURL = string(du)
			}
		}
	}
	linkURL := downloadURL
	// NoRedirect GET with Referer.
	hc := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", ua)
	resp2, err := hc.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp2.Body.Close()
	switch {
	case resp2.StatusCode == http.StatusFound: // 302
		loc := resp2.Header.Get("Location")
		if loc != "" {
			linkURL = loc
		}
	case resp2.StatusCode < 300:
		body, _ := io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
		if redirect := extractPan123RedirectURL(string(body), downloadURL); redirect != "" {
			linkURL = redirect
		}
	}
	return linkURL, map[string]string{"Referer": referer}, nil
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	f, err := d.resolveAListFile(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	if f.Type == 1 {
		return nil, errors.New("文件夹不能直接下载")
	}
	if f.S3KeyFlag == "" {
		return nil, errors.New("File.S3KeyFlag 为空（列表未返回，与 AList 所需 File 不一致）")
	}
	linkURL, headers, err := d.alistLink(ctx, c, *f)
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID,
		ExpireTime:   getExpiresTime(linkURL),
		URL:          linkURL,
		Size:         f.Size,
		Headers:      headers,
		DownloadMode: "proxy", ForceLocalProxy: true, Concurrency: 1,
	}, nil
}

// GetVideoPreview reuses the download source as an origin-quality playback
// stream (the legacy client had no dedicated preview endpoint; proxy playback).
func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 0)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID, FileID: fileID, Size: u.Size, Headers: u.Headers,
		Qualities: []model.VideoQuality{{
			Quality: "origin", Label: "原画", Value: "origin", URL: u.URL,
			Headers: u.Headers, ForceProxy: true,
		}},
	}, nil
}

// ---- file commands (legacy filecmd.ts) ----

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	res := &drive.MkdirResult{}
	resp, err := d.api(ctx, c, http.MethodPost, apiUploadReq, map[string]any{
		"driveId":      0,
		"etag":         "",
		"fileName":     name,
		"parentFileId": toPan123Number(parentID),
		"size":         0,
		"type":         1,
	}, nil)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	data := parseMap(resp.Data)
	res.FileID = firstString(data, "FileId", "fileId")
	return res, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	_, err := d.api(ctx, c, http.MethodPost, apiRename, map[string]any{
		"driveId":  0,
		"fileId":   toPan123Number(fileID),
		"fileName": name,
	}, nil)
	if err != nil {
		return nil, err
	}
	result := &drive.RenameResult{FileID: fileID, Name: name}
	if detail, err := d.GetFile(ctx, c, fileID); err == nil {
		result.ParentFileID = detail.ParentFileID
		result.IsDir = detail.IsDir
	}
	return result, nil
}

// batchOp applies fn to each id, collecting the successful ones (legacy skips
// per-item failures).
func (d *Driver) batchOp(ctx context.Context, c drive.Context, ids []string, fn func(ctx context.Context, c drive.Context, id int64) error) ([]string, error) {
	var ok []string
	for _, id := range ids {
		if err := fn(ctx, c, toPan123Number(id)); err != nil {
			continue
		}
		ok = append(ok, id)
	}
	return ok, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return d.batchOp(ctx, c, fileIDs, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiTrash, map[string]any{
			"driveId":           0,
			"operation":         true,
			"fileTrashInfoList": []any{map[string]any{"FileId": id}},
		}, nil)
		return err
	})
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return d.batchOp(ctx, c, fileIDs, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiTrash, map[string]any{
			"driveId":           0,
			"operation":         false,
			"fileTrashInfoList": []any{map[string]any{"FileId": id}},
		}, nil)
		return err
	})
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	return d.batchOp(ctx, c, ids, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiDelete, map[string]any{
			"driveId":           0,
			"operation":         true,
			"fileTrashInfoList": []any{map[string]any{"FileId": id}},
		}, nil)
		return err
	})
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	parent := toPan123Number(toParentID)
	return d.batchOp(ctx, c, ids, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiMove, map[string]any{
			"fileIdList":   []any{map[string]any{"FileId": id}},
			"parentFileId": parent,
		}, nil)
		return err
	})
}

// Copy: AList 123 web 不支持服务端复制（legacy copy 直接返回空数组）。
func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	return []string{}, nil
}

// ---- share (legacy share.ts) ----

// formatPan123Expiration renders expiration as local 'yyyy-MM-dd HH:mm:ss'.
func formatPan123Expiration(value string) string {
	if value == "" {
		return ""
	}
	t := parseFlexibleTime(value)
	if t.IsZero() {
		return ""
	}
	return formatPan123Time(t)
}

// formatPan123Time renders a time in local wall clock yyyy-MM-dd HH:mm:ss.
func formatPan123Time(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04:05")
}

// parseLocalTime accepts ISO-8601 and common local layouts. Zone-less strings
// are parsed in the local location to mirror JS Date parsing (new Date(str)).
func parseLocalTime(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		var (
			t   time.Time
			err error
		)
		if layout == time.RFC3339 {
			t, err = time.Parse(layout, value)
		} else {
			t, err = time.ParseInLocation(layout, value, time.Local)
		}
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseFlexibleTime accepts ISO-8601 and common local layouts.
func parseFlexibleTime(value string) time.Time {
	return parseLocalTime(value)
}

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	shareName := params.ShareName
	if shareName == "" {
		shareName = "分享文件"
	}
	fileIDs := make([]int64, 0, len(params.FileIDs))
	for _, id := range params.FileIDs {
		fileIDs = append(fileIDs, toPan123Number(id))
	}
	body := map[string]any{
		"driveId":            0,
		"fileIdList":         fileIDs,
		"displayStatus":      1,
		"expirationTime":     formatPan123Expiration(params.Expiration),
		"isReward":           false,
		"isEvent":            false,
		"event":              "shareCreateFile",
		"shareName":          shareName,
		"sharePwd":           params.Password,
		"trafficLimit":       0,
		"trafficLimitSwitch": false,
		"renamable":          false,
		"renameMode":         0,
	}
	resp, err := d.api(ctx, c, http.MethodPost, apiShareURL, body, nil)
	if err != nil {
		return nil, err
	}
	data := parseMap(resp.Data)
	shareKey := firstString(data, "ShareKey", "ShareId")
	shareURL := firstString(data, "ShareUrl", "shareUrl")
	if shareURL == "" && shareKey != "" {
		shareURL = "https://www.123pan.com/s/" + shareKey
	}
	if shareURL == "" {
		return nil, errors.New("创建分享失败：未返回链接")
	}
	pwd := firstString(data, "SharePwd", "sharePwd")
	if pwd == "" {
		pwd = params.Password
	}
	item := &model.ShareItem{
		ShareID:    shareKey,
		ShareURL:   shareURL,
		SharePwd:   pwd,
		ShareName:  shareName,
		Expiration: params.Expiration,
		DriveID:    c.DriveID,
		FileID:     params.FileIDs[0],
		FileIDList: params.FileIDs,
		ShareMsg:   "创建成功",
	}
	return item, nil
}

// ---- rapid upload / transfer hash (legacy rapidUpload.ts) ----

// etagAsMd5 accepts an etag only when it is a plain 32-hex MD5.
func etagAsMd5(etag string) string {
	v := strings.TrimSpace(etag)
	v = strings.ToLower(v)
	if md5Re.MatchString(v) {
		return v
	}
	return ""
}

var md5Re = regexp.MustCompile(`^[a-f0-9]{32}$`)

// duplicateFromRequest maps the unified duplicate policy to the 123 value:
// 2=overwrite, anything else → 1 (keep both / rename; 123 has no skip value).
func duplicateFromRequest(duplicate int) int {
	if duplicate == 2 {
		return 2
	}
	return 1
}

func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if req.Method != "md5" {
		return &drive.RapidUploadResult{Reuse: false, Message: "123 仅支持 MD5 秒传"}, nil
	}
	etag := etagAsMd5(req.Hash)
	if etag == "" {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 MD5 指纹"}, nil
	}
	resp, err := d.api(ctx, c, http.MethodPost, apiUploadReq, map[string]any{
		"driveId":      0,
		"duplicate":    duplicateFromRequest(req.Duplicate),
		"etag":         etag,
		"fileName":     req.FileName,
		"parentFileId": toPan123Number(req.ParentID),
		"size":         req.Size,
		"type":         0,
	}, nil)
	if err != nil {
		return nil, err
	}
	data := parseMap(resp.Data)
	reuse := asBool(pick(data, "Reuse", "reuse"))
	fileID := firstString(data, "FileId", "fileId")
	key := firstString(data, "Key", "key")
	if reuse || (fileID != "" && key == "") {
		return &drive.RapidUploadResult{Reuse: true, FileID: fileID, ParentID: req.ParentID, Message: "秒传命中"}, nil
	}
	return &drive.RapidUploadResult{Reuse: false, Message: "未命中秒传"}, nil
}

func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if method != "md5" {
		return "", nil
	}
	fid := toPan123FileID(fileID)
	if pooled, ok := poolGet(fid); ok && pooled.Etag != "" {
		return etagAsMd5(pooled.Etag), nil
	}
	detail, err := d.detail(ctx, c, fileID)
	if err != nil {
		return "", nil
	}
	return etagAsMd5(detail.Etag), nil
}

// ---- driver ----

// Driver implements drive.Driver for 123 云盘.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return RootID }
