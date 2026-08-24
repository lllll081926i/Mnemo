package pan123

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
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
)

var (
	poolMu   sync.Mutex
	filePool = map[string]pan123File{}
)

// putPool normalizes and stores a file; non-empty fields win, but empty
// S3KeyFlag/Etag/FileName from a stub /file/info never poison a listed file.
func poolKey(driveID, fileID string) string {
	return driveID + "\x00" + toPan123FileID(fileID)
}

func putPool(c drive.Context, f pan123File) pan123File {
	poolMu.Lock()
	defer poolMu.Unlock()
	id := toPan123FileID(f.FileID)
	if id == "" || id == "0" {
		return f
	}
	key := poolKey(c.DriveID, id)
	if ex, ok := filePool[key]; ok {
		f = mergeFile(f, ex)
	}
	delete(filePool, key)
	filePool[key] = f
	for len(filePool) > maxFilePool {
		for k := range filePool {
			delete(filePool, k)
			break
		}
	}
	return f
}

func poolGet(c drive.Context, fileID string) (pan123File, bool) {
	poolMu.Lock()
	defer poolMu.Unlock()
	f, ok := filePool[poolKey(c.DriveID, fileID)]
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

// pan123FileFromModel restores the provider-only fields from a unified list
// snapshot. The API detail endpoint does not always return S3KeyFlag, while
// the list response does; Description is the durable bridge between them.
func pan123FileFromModel(f model.File) (pan123File, bool) {
	if strings.TrimSpace(f.FileID) == "" {
		return pan123File{}, false
	}
	item, ok := decodePan123MetaDesc(f.Description)
	if !ok || item.S3KeyFlag == "" {
		return pan123File{}, false
	}
	item.FileID = toPan123FileID(f.FileID)
	if item.FileName == "" {
		item.FileName = f.Name
	}
	if item.Size == 0 {
		item.Size = f.Size
	}
	if item.Etag == "" {
		item.Etag = f.ContentHash
	}
	if item.ParentFileID == "" {
		item.ParentFileID = f.ParentFileID
	}
	if f.IsDir {
		item.Type = 1
	}
	return item, true
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

// fileListPageRaw returns one page from the 123 API. The API uses a numeric
// Page field and reports "-1" in Next for the final page.
func (d *Driver) fileListPageRaw(ctx context.Context, c drive.Context, parentID string, trashed bool, search string, page int) ([]pan123File, string, error) {
	parentFileID := toPan123FileID(parentID)
	if page < 1 {
		page = 1
	}
	event := "homeListFile"
	operateType := "4"
	if search != "" {
		event = "homeSearchFile"
		operateType = "2"
	}
	query := map[string]string{
		"driveId":              "0",
		"limit":                listPageLimit,
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
		return nil, "", err
	}
	data := parseMap(resp.Data)
	list := rawList(data)
	res := make([]pan123File, 0, len(list))
	for _, raw := range list {
		if m, ok := raw.(map[string]any); ok {
			res = append(res, putPool(c, normalizePan123File(m)))
		}
	}
	next := asString(pick(data, "Next", "next"))
	if len(list) == 0 || next == "-1" {
		next = ""
	} else {
		next = strconv.Itoa(page + 1)
	}
	return res, next, nil
}

// fileListRaw implements the AList getFiles paging loop (guard 200 pages).
func (d *Driver) fileListRaw(ctx context.Context, c drive.Context, parentID string, trashed bool, search string) ([]pan123File, error) {
	var res []pan123File
	marker := "1"
	for guard := 0; guard < 200; guard++ {
		page, next, err := d.fileListPageRaw(ctx, c, parentID, trashed, search, pageMarker123(marker))
		if err != nil {
			return nil, err
		}
		res = append(res, page...)
		if next == "" {
			return res, nil
		}
		marker = next
	}
	return nil, errors.New("123: 文件列表分页超过上限")
}

func pageMarker123(marker string) int {
	page, err := strconv.Atoi(marker)
	if err != nil || page < 1 {
		return 1
	}
	return page
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

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, opts *drive.ListOptions) (*drive.DirPage, error) {
	parent := toPan123FileID(dirID)
	search := ""
	if opts != nil {
		search = opts.Search
	}
	page, next, err := d.fileListPageRaw(ctx, c, parent, false, search, pageMarker123(marker))
	if err != nil {
		return nil, err
	}
	return &drive.DirPage{Items: mapFiles(page, c.DriveID, parent), NextMarker: next}, nil
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
	f := putPool(c, normalizePan123File(m))
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
	if pooled, ok := poolGet(c, fid); ok {
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
// S3KeyFlag. 顺序对齐旧版 resolvePan123AListFile，并优先消费统一缓存中
// 的点击快照：缓存 → pool → 详情 → 父目录重拉 → 根目录重拉 → 文件名搜索。
func (d *Driver) resolveAListFile(ctx context.Context, c drive.Context, fileID string) (*pan123File, error) {
	fid := toPan123FileID(fileID)
	// A download/preview may be triggered from a cached frontend row after
	// the provider's in-memory pool has been evicted. Prefer that exact row
	// snapshot so the list-only S3KeyFlag survives the transition.
	if cached, ok := drive.CachedFile(c.UserID, c.DriveID, fid); ok {
		if restored, ok := pan123FileFromModel(cached); ok {
			pinned := putPool(c, restored)
			return &pinned, nil
		}
	}
	if f, ok := poolGet(c, fid); ok && f.S3KeyFlag != "" {
		return &f, nil
	}
	// 先尝试直接请求 /file/info 详情接口补全
	if det, err := d.detail(ctx, c, fileID); err == nil && det != nil && det.S3KeyFlag != "" {
		return det, nil
	}
	// 冷启动/池被驱逐：用池里残条的父目录与文件名做线索
	var parentID, name string
	if stub, ok := poolGet(c, fid); ok {
		parentID, name = stub.ParentFileID, stub.FileName
	}
	if parentID != "" && parentID != "0" {
		if items, err := d.fileListRaw(ctx, c, parentID, false, ""); err == nil {
			for i := range items {
				if items[i].FileID == fid && items[i].S3KeyFlag != "" {
					return &items[i], nil
				}
			}
		}
	}
	if items, err := d.fileListRaw(ctx, c, "0", false, ""); err == nil {
		for i := range items {
			if items[i].FileID == fid && items[i].S3KeyFlag != "" {
				return &items[i], nil
			}
		}
	}
	// 全局搜索兜底：按文件名搜（旧版关键词为文件名，fid 搜不到）
	kw := name
	if kw == "" {
		kw = fid
	}
	if items, err := d.fileListRaw(ctx, c, "0", false, kw); err == nil {
		for i := range items {
			if items[i].FileID == fid && items[i].S3KeyFlag != "" {
				return &items[i], nil
			}
		}
	}
	if f, ok := poolGet(c, fid); ok && f.S3KeyFlag != "" {
		return &f, nil
	}
	return nil, errors.New("can't convert obj（无法获取 123 盘下载直链，请先刷新所在文件夹）")
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

// decodePan123ParamsURL mirrors the legacy Buffer.from(value, "base64")
// behaviour while restricting the result to a usable HTTP(S) URL. The API
// has returned standard padded, raw and URL-safe variants over time.
func decodePan123ParamsURL(raw string) string {
	variants := []string{raw}
	if strings.ContainsAny(raw, " \t\r\n") {
		withoutWhitespace := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '\r', '\n':
				return -1
			default:
				return r
			}
		}, raw)
		variants = append(variants, withoutWhitespace, strings.ReplaceAll(raw, " ", "+"))
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, value := range variants {
		for _, encoding := range encodings {
			decoded, err := encoding.DecodeString(value)
			if err != nil || len(decoded) == 0 {
				continue
			}
			u, err := url.Parse(string(decoded))
			if err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
				return u.String()
			}
		}
	}
	return ""
}

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
			if decodedURL := decodePan123ParamsURL(nu); decodedURL != "" {
				downloadURL = decodedURL
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
