package pan139

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

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

func pan139PrecomputedSHA256(info model.UploadInfo) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(info.ContentHashAlgorithm), "sha256") {
		return "", false
	}
	value := strings.ToLower(strings.TrimSpace(info.ContentHash))
	if len(value) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return value, true
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
	contentHash, precomputed := pan139PrecomputedSHA256(ui.Info)
	if !precomputed {
		contentHash, err = hashPan139File(ctx, f, ui)
		if err != nil {
			return err
		}
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
// download and upload; the pending precreate session is persisted for that
// uploader to resume.
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
			if err := drive.SaveUploadSessionState(key, encodePan139UploadSession(pan139UploadSession{
				FileID: created.FileID.String(), UploadID: created.UploadID, ContentHash: hashValue,
			}), nil); err != nil {
				if _, cleanupErr := d.Delete(ctx, c, []drive.FileRef{{ID: created.FileID.String()}}); cleanupErr != nil {
					return nil, errors.Join(
						fmt.Errorf("pan139: 保存秒传未命中会话: %w", err),
						fmt.Errorf("pan139: 清理秒传未命中任务 %s: %w", created.FileID.String(), cleanupErr),
					)
				}
				return nil, fmt.Errorf("pan139: 保存秒传未命中会话: %w", err)
			}
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
