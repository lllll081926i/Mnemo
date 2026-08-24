package aliopen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type aliOpenUploadPart struct {
	PartNumber int    `json:"part_number"`
	UploadURL  string `json:"upload_url"`
}

type aliOpenUploadCreateOptions struct {
	PartSize        int64
	ContentHash     string
	PreHash         string
	ProofCode       string
	CheckNameMode   string
	LocalModifiedAt string
	LocalCreatedAt  string
}

type aliOpenUploadCreateResult struct {
	FileID       string              `json:"file_id"`
	UploadID     string              `json:"upload_id"`
	PartInfoList []aliOpenUploadPart `json:"part_info_list"`
	RapidUpload  bool                `json:"rapid_upload"`
	Exist        bool                `json:"exist"`
}

func aliOpenPartSize(size int64) int64 {
	switch {
	case size > 1024*aliOpenGiB:
		return 5 * aliOpenGiB
	case size > 768*aliOpenGiB:
		return 109951163
	case size > 512*aliOpenGiB:
		return 82463373
	case size > 384*aliOpenGiB:
		return 54975582
	case size > 256*aliOpenGiB:
		return 41231687
	case size > 128*aliOpenGiB:
		return 27487791
	default:
		return 20 * aliOpenMiB
	}
}

func aliOpenPartCount(size, partSize int64) int {
	if size <= 0 || partSize <= 0 {
		return 1
	}
	return int((size-1)/partSize + 1)
}

func aliOpenCheckNameMode(policy string) string {
	switch driveutil.ResolveConflictPolicy(policy) {
	case driveutil.ConflictRefuse:
		return "fail"
	case driveutil.ConflictRename:
		return "auto_rename"
	case driveutil.ConflictSkip:
		// Skip is checked before create. Keep the API conservative if a
		// concurrent writer creates the same name between those two calls.
		return "fail"
	default:
		return "ignore"
	}
}

func aliOpenCheckNameModeForDuplicate(duplicate int) string {
	switch duplicate {
	case 2:
		return "ignore"
	case 1:
		return "fail"
	default:
		return "auto_rename"
	}
}

func (c *client) hasNamedFile(ctx context.Context, scope Scope, parentID, name string) (bool, error) {
	items, err := c.List(ctx, scope, parentID)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// RapidUpload attempts sha1-based rapid upload (秒传).
func (c *client) RapidUpload(ctx context.Context, scope Scope, parentID, name string, size int64, sha1Str string) (*drive.RapidUploadResult, error) {
	return c.rapidUploadWithMode(ctx, scope, parentID, name, size, sha1Str, "ignore")
}

func (c *client) rapidUploadWithMode(ctx context.Context, scope Scope, parentID, name string, size int64, sha1Str, checkNameMode string) (*drive.RapidUploadResult, error) {
	var res struct {
		FileID       string `json:"file_id"`
		RapidUpload  bool   `json:"rapid_upload"`
		PartInfoList []struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		} `json:"part_info_list"`
		Exist bool   `json:"exist"`
		Error string `json:"error"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":              name,
		"parent_file_id":    parentID,
		"drive_id":          c.scopedDriveID(scope),
		"type":              "file",
		"size":              size,
		"content_hash":      sha1Str,
		"content_hash_name": "sha1",
		"check_name_mode":   checkNameMode,
	}, &res); err != nil {
		return nil, err
	}
	// A miss still returns file_id for the pending upload object. Only the
	// explicit rapid_upload flag means the remote file is already complete.
	if res.RapidUpload || res.Exist {
		if strings.TrimSpace(res.FileID) == "" {
			return &drive.RapidUploadResult{Reuse: false, Message: "秒传响应缺少 file_id"}, nil
		}
		return &drive.RapidUploadResult{Reuse: true, FileID: res.FileID}, nil
	}
	// A hash miss may still allocate a pending remote file. This probe does not
	// retain enough multipart state for the normal uploader, so remove the
	// pending entry before migration falls back and creates a resumable session.
	if pendingID := strings.TrimSpace(res.FileID); pendingID != "" {
		if err := c.Delete(ctx, scope, pendingID); err != nil {
			return nil, fmt.Errorf("aliopen: 清理秒传未命中任务 %s: %w", pendingID, err)
		}
	}
	// Do not expose the pending file id: the migration layer treats a file id
	// as an accepted transfer for compatibility with providers that return one
	// on a successful probe.
	return &drive.RapidUploadResult{Reuse: false, Message: "秒传未命中，需要上传"}, nil
}

// CreateUploadFile creates an upload entry and returns parts.
func (c *client) CreateUploadFile(ctx context.Context, scope Scope, parentID, name string, size int64, options aliOpenUploadCreateOptions) (aliOpenUploadCreateResult, error) {
	partSize := options.PartSize
	if partSize <= 0 {
		partSize = aliOpenPartSize(size)
	}
	partCount := aliOpenPartCount(size, partSize)
	partInfoList := make([]map[string]int, partCount)
	for i := range partInfoList {
		partInfoList[i] = map[string]int{"part_number": i + 1}
	}
	mode := strings.TrimSpace(options.CheckNameMode)
	if mode == "" {
		mode = "ignore"
	}
	body := map[string]any{
		"name":            name,
		"parent_file_id":  parentID,
		"drive_id":        c.scopedDriveID(scope),
		"type":            "file",
		"size":            size,
		"check_name_mode": mode,
		"part_info_list":  partInfoList,
	}
	if hash := strings.TrimSpace(options.ContentHash); hash != "" {
		body["content_hash"] = strings.ToUpper(hash)
		body["content_hash_name"] = "sha1"
	}
	if preHash := strings.TrimSpace(options.PreHash); preHash != "" {
		body["pre_hash"] = strings.ToUpper(preHash)
	}
	if proof := strings.TrimSpace(options.ProofCode); proof != "" {
		body["proof_version"] = "v1"
		body["proof_code"] = proof
	}
	if value := strings.TrimSpace(options.LocalModifiedAt); value != "" {
		body["local_modified_at"] = value
	}
	if value := strings.TrimSpace(options.LocalCreatedAt); value != "" {
		body["local_created_at"] = value
	}
	var result aliOpenUploadCreateResult
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", body, &result); err != nil {
		return aliOpenUploadCreateResult{}, err
	}
	return result, nil
}

func (c *client) fillUploadURLs(ctx context.Context, scope Scope, fileID, uploadID string, parts []aliOpenUploadPart, uploaded map[int]bool) error {
	missing := make([]map[string]int, 0, len(parts))
	for _, part := range parts {
		if !uploaded[part.PartNumber] && strings.TrimSpace(part.UploadURL) == "" {
			missing = append(missing, map[string]int{"part_number": part.PartNumber})
		}
	}
	for start := 0; start < len(missing); start += 100 {
		end := start + 100
		if end > len(missing) {
			end = len(missing)
		}
		var refreshed struct {
			PartInfoList []aliOpenUploadPart `json:"part_info_list"`
		}
		if err := c.apiPost(ctx, "/adrive/v1.0/openFile/getUploadUrl", map[string]any{
			"drive_id":       c.scopedDriveID(scope),
			"file_id":        fileID,
			"upload_id":      uploadID,
			"part_info_list": missing[start:end],
		}, &refreshed); err != nil {
			return err
		}
		urls := make(map[int]string, len(refreshed.PartInfoList))
		for _, part := range refreshed.PartInfoList {
			urls[part.PartNumber] = strings.TrimSpace(part.UploadURL)
		}
		for i := range parts {
			if url := urls[parts[i].PartNumber]; url != "" {
				parts[i].UploadURL = url
			}
		}
	}
	for _, part := range parts {
		if !uploaded[part.PartNumber] && strings.TrimSpace(part.UploadURL) == "" {
			return fmt.Errorf("aliopen: 分片 %d 无上传地址", part.PartNumber)
		}
	}
	return nil
}

func aliOpenPreHash(f *os.File, size int64) (string, error) {
	if f == nil || size <= 0 {
		return "", nil
	}
	length := size
	if length > 1024 {
		length = 1024
	}
	buf := make([]byte, int(length))
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.ToUpper(netx.SHA1Hex(buf[:n])), nil
}

func aliOpenProofCode(f *os.File, accessToken string, size int64) (string, error) {
	if f == nil || strings.TrimSpace(accessToken) == "" || size <= 0 {
		return "", nil
	}
	digest := netx.MD5Hex([]byte(accessToken))
	position, err := strconv.ParseUint(digest[:16], 16, 64)
	if err != nil {
		return "", err
	}
	start := int64(position % uint64(size))
	length := int64(8)
	if remain := size - start; remain < length {
		length = remain
	}
	buf := make([]byte, int(length))
	n, err := f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf[:n]), nil
}

func aliOpenLocalFileTime(info os.FileInfo) string {
	if info == nil || info.ModTime().IsZero() {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339Nano)
}

func aliOpenIsPreHashMatched(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "prehashmatched") || strings.Contains(message, "pre_hash_matched")
}

func encodeAliOpenUploadSession(uploadID, fileID string) string {
	b, _ := json.Marshal(map[string]string{"upload_id": uploadID, "file_id": fileID})
	return string(b)
}

func decodeAliOpenUploadSession(raw string) (uploadID, fileID string) {
	var state struct {
		UploadID string `json:"upload_id"`
		FileID   string `json:"file_id"`
	}
	if json.Unmarshal([]byte(raw), &state) == nil {
		return strings.TrimSpace(state.UploadID), strings.TrimSpace(state.FileID)
	}
	// Older builds persisted only upload_id. It cannot be completed without
	// file_id, so the caller will discard that stale state and create anew.
	return strings.TrimSpace(raw), ""
}

// CompleteUpload marks the upload as complete.
func (c *client) CompleteUpload(ctx context.Context, scope Scope, fileID, uploadID string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/complete", map[string]any{
		"file_id":   fileID,
		"drive_id":  c.scopedDriveID(scope),
		"upload_id": uploadID,
	}, nil)
}

// ResolveHash extracts the sha1 from a file's metadata.
func (c *client) ResolveHash(ctx context.Context, scope Scope, fileID string) (string, error) {
	file, err := c.Detail(ctx, scope, fileID)
	if err != nil {
		return "", err
	}
	hashValue := strings.ToLower(strings.TrimSpace(file.ContentHash))
	if strings.EqualFold(strings.TrimSpace(file.ContentHashName), "sha1") && regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(hashValue) {
		return hashValue, nil
	}
	return "", nil
}

// UploadOneFile uploads one local file with resumable multipart support.
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil || strings.TrimSpace(ui.Info.LocalFilePath) == "" {
		return errors.New("aliopen: 上传文件路径为空")
	}
	ref := parseRef(ui.Info.ParentFileID)
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, _ := f.Stat()
	if info == nil {
		return errors.New("aliopen: stat file failed")
	}
	size := info.Size()
	ui.Info.Size = size
	policy := driveutil.ResolveConflictPolicy(ui.Info.ConflictPolicy)
	if policy == driveutil.ConflictSkip {
		exists, err := cl.hasNamedFile(ctx, ref.Scope, ref.FID, ui.Info.Name)
		if err != nil {
			return err
		}
		if exists {
			ui.ReportUploadProgress(size, size)
			return nil
		}
	}

	contentHash := strings.ToUpper(strings.TrimSpace(ui.Info.SHA1))
	if contentHash == "" {
		contentHash, err = netx.HashFileWithProgress(ui.Info.LocalFilePath, netx.HashSHA1, func(read int64) {
			if ui != nil {
				ui.ReportUploadProgress(read, ui.Info.Size)
			}
		}, 0)
		if err != nil {
			return fmt.Errorf("aliopen: 计算文件 SHA1 失败: %w", err)
		}
		contentHash = strings.ToUpper(contentHash)
		ui.Info.SHA1 = contentHash
	}
	partSize := aliOpenPartSize(size)
	partCount := aliOpenPartCount(size, partSize)
	sessionKey := drive.UploadSessionKey(c.UserID, c.DriveID, ui.Info.ParentFileID, ui.Info.Name, size)
	savedSessionID, savedParts := drive.LoadUploadSessionState(sessionKey)
	uploadedSet := make(map[int]bool)
	fileID, uploadID := decodeAliOpenUploadSession(savedSessionID)
	if fileID == "" || uploadID == "" {
		fileID, uploadID = "", ""
	} else {
		for _, pn := range savedParts {
			if pn >= 1 && pn <= partCount {
				uploadedSet[pn] = true
			}
		}
	}

	parts := make([]aliOpenUploadPart, partCount)
	for i := range parts {
		parts[i].PartNumber = i + 1
	}
	if fileID == "" || uploadID == "" {
		preHash, preHashErr := aliOpenPreHash(f, size)
		if preHashErr != nil {
			return fmt.Errorf("aliopen: 计算 pre_hash 失败: %w", preHashErr)
		}
		localTime := aliOpenLocalFileTime(info)
		options := aliOpenUploadCreateOptions{
			PartSize:        partSize,
			ContentHash:     contentHash,
			PreHash:         preHash,
			CheckNameMode:   aliOpenCheckNameMode(ui.Info.ConflictPolicy),
			LocalModifiedAt: localTime,
			LocalCreatedAt:  localTime,
		}
		created, createErr := cl.CreateUploadFile(ctx, ref.Scope, ref.FID, ui.Info.Name, size, options)
		if createErr != nil && !aliOpenIsPreHashMatched(createErr) {
			return createErr
		}
		if createErr != nil || ((!created.RapidUpload && !created.Exist) && (created.FileID == "" || created.UploadID == "")) {
			proofCode, proofErr := aliOpenProofCode(f, cl.session.AccessToken, size)
			if proofErr != nil {
				return fmt.Errorf("aliopen: 计算 proof_code 失败: %w", proofErr)
			}
			options.PreHash = ""
			options.ProofCode = proofCode
			created, createErr = cl.CreateUploadFile(ctx, ref.Scope, ref.FID, ui.Info.Name, size, options)
			if createErr != nil {
				return createErr
			}
		}
		if created.RapidUpload || created.Exist {
			if strings.TrimSpace(created.FileID) == "" {
				return errors.New("aliopen: 秒传响应缺少 file_id")
			}
			ui.Upload.FileID = created.FileID
			ui.ReportUploadProgress(size, size)
			return nil
		}
		fileID, uploadID = strings.TrimSpace(created.FileID), strings.TrimSpace(created.UploadID)
		if fileID == "" || uploadID == "" {
			return errors.New("aliopen: 创建上传任务未返回 file_id 或 upload_id")
		}
		parts = created.PartInfoList
	}

	// The persisted record stores only the remote session id and completed
	// parts. Rebuild part metadata on resume because pre-signed URLs expire.
	byNumber := make(map[int]aliOpenUploadPart, len(parts))
	for _, part := range parts {
		if part.PartNumber >= 1 && part.PartNumber <= partCount {
			byNumber[part.PartNumber] = part
		}
	}
	parts = make([]aliOpenUploadPart, partCount)
	for i := range parts {
		parts[i].PartNumber = i + 1
		if part, ok := byNumber[i+1]; ok {
			parts[i].UploadURL = part.UploadURL
		}
	}
	// Persist the remote session before requesting pre-signed URLs so a
	// transient URL request failure can still resume the same upload task.
	_ = drive.SaveUploadSessionState(sessionKey, encodeAliOpenUploadSession(uploadID, fileID), drive.SortedUniqueParts(uploadedSet))
	if err := cl.fillUploadURLs(ctx, ref.Scope, fileID, uploadID, parts, uploadedSet); err != nil {
		return err
	}
	ui.Upload.FileID = fileID

	var uploadedSize int64
	for partNumber := range uploadedSet {
		start := int64(partNumber-1) * partSize
		if start >= size {
			continue
		}
		length := partSize
		if remain := size - start; remain < length {
			length = remain
		}
		uploadedSize += length
	}
	ui.ReportUploadProgress(uploadedSize, size)

	lastURLRefresh := time.Now()
	for i := range parts {
		part := &parts[i]
		start := int64(part.PartNumber-1) * partSize
		length := int64(0)
		if start < size {
			length = partSize
			if remain := size - start; remain < length {
				length = remain
			}
		}
		if !uploadedSet[part.PartNumber] && part.UploadURL != "" {
			if time.Since(lastURLRefresh) >= 50*time.Minute {
				var refreshed struct {
					PartInfoList []aliOpenUploadPart `json:"part_info_list"`
				}
				if refreshErr := cl.apiPost(ctx, "/adrive/v1.0/openFile/getUploadUrl", map[string]any{
					"drive_id":       cl.scopedDriveID(ref.Scope),
					"file_id":        fileID,
					"upload_id":      uploadID,
					"part_info_list": []map[string]int{{"part_number": part.PartNumber}},
				}, &refreshed); refreshErr != nil || len(refreshed.PartInfoList) == 0 || refreshed.PartInfoList[0].UploadURL == "" {
					if refreshErr != nil {
						return refreshErr
					}
					return fmt.Errorf("aliopen: 分片 %d 刷新上传地址失败", part.PartNumber)
				}
				part.UploadURL = strings.TrimSpace(refreshed.PartInfoList[0].UploadURL)
				lastURLRefresh = time.Now()
			}
			putPart := func(uploadURL string) (int, error) {
				body := io.NewSectionReader(f, start, length)
				req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, body)
				if err != nil {
					return 0, err
				}
				req.ContentLength = length
				req.Header.Set("Content-Length", strconv.FormatInt(length, 10))
				resp, err := cl.http.HTTP.Do(req)
				if err != nil {
					return 0, err
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				status := resp.StatusCode
				resp.Body.Close()
				return status, nil
			}
			status, err := putPart(part.UploadURL)
			if err != nil {
				return err
			}
			if (status == http.StatusUnauthorized || status == http.StatusForbidden) && status >= 400 {
				// A persisted session can contain expired pre-signed URLs. Refresh
				// only the current part so a recoverable expiry does not discard the
				// whole upload session.
				var refreshed struct {
					PartInfoList []aliOpenUploadPart `json:"part_info_list"`
				}
				if refreshErr := cl.apiPost(ctx, "/adrive/v1.0/openFile/getUploadUrl", map[string]any{
					"drive_id":       cl.scopedDriveID(ref.Scope),
					"file_id":        fileID,
					"upload_id":      uploadID,
					"part_info_list": []map[string]int{{"part_number": part.PartNumber}},
				}, &refreshed); refreshErr == nil && len(refreshed.PartInfoList) > 0 && refreshed.PartInfoList[0].UploadURL != "" {
					part.UploadURL = strings.TrimSpace(refreshed.PartInfoList[0].UploadURL)
					status, err = putPart(part.UploadURL)
					if err != nil {
						return err
					}
				}
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("aliopen: 分片上传失败 HTTP %d", status)
			}
		}
		// Persist uploaded part number incrementally
		if !uploadedSet[part.PartNumber] {
			uploadedSet[part.PartNumber] = true
			_ = drive.SaveUploadSessionState(sessionKey, encodeAliOpenUploadSession(uploadID, fileID), drive.SortedUniqueParts(uploadedSet))
		}
		uploadedSize += length
		if ui != nil {
			ui.ReportUploadProgress(uploadedSize, size)
		}
	}
	err = cl.CompleteUpload(ctx, ref.Scope, fileID, uploadID)
	if err == nil {
		drive.ClearUploadSession(sessionKey)
	}
	return err
}

func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if req.Method != "sha1" {
		return &drive.RapidUploadResult{Reuse: false, Message: "阿里云盘仅支持 SHA1 秒传"}, nil
	}
	hashValue := strings.ToLower(strings.TrimSpace(req.Hash))
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(hashValue) {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 SHA1 指纹"}, nil
	}
	if req.Size < 0 {
		return &drive.RapidUploadResult{Reuse: false, Message: "文件大小不能为负数"}, nil
	}
	ref := parseRef(req.ParentID)
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	if req.Duplicate == 1 {
		exists, err := cl.hasNamedFile(ctx, ref.Scope, ref.FID, req.FileName)
		if err != nil {
			return nil, err
		}
		if exists {
			return &drive.RapidUploadResult{Reuse: true, Message: "目标文件已存在，已跳过"}, nil
		}
	}
	return cl.rapidUploadWithMode(ctx, ref.Scope, ref.FID, req.FileName, req.Size, hashValue, aliOpenCheckNameModeForDuplicate(req.Duplicate))
}

func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if method != "sha1" {
		return "", nil
	}
	ref := parseRef(fileID)
	cl, err := clientOf(c)
	if err != nil {
		return "", err
	}
	return cl.ResolveHash(ctx, ref.Scope, ref.FID)
}
