package pan123

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
)

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

// duplicateFromPolicy mirrors the legacy pan123DuplicateFromPolicy mapping:
// 123 accepts 2 for overwrite and 1 for the non-overwrite branch. The API
// does not expose separate native values for rename and skip.
func duplicateFromPolicy(policy string) int {
	if driveutil.ResolveConflictPolicy(policy) == driveutil.ConflictOverwrite {
		return 2
	}
	return 1
}

func rapidUploadParentID(parentID string) (int64, bool) {
	value := toPan123FileID(parentID)
	if value == "0" {
		return 0, true
	}
	parent, err := strconv.ParseInt(value, 10, 64)
	return parent, err == nil && parent > 0
}

func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if req.Method != "md5" {
		return &drive.RapidUploadResult{Reuse: false, Message: "123 仅支持 MD5 秒传"}, nil
	}
	etag := etagAsMd5(req.Hash)
	if etag == "" {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 MD5 指纹"}, nil
	}
	if req.Size < 0 {
		return &drive.RapidUploadResult{Reuse: false, Message: "文件大小不能为负数"}, nil
	}
	parentID, ok := rapidUploadParentID(req.ParentID)
	if !ok {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的目标目录 ID"}, nil
	}
	resp, err := d.api(ctx, c, http.MethodPost, apiUploadReq, map[string]any{
		"driveId":      0,
		"duplicate":    duplicateFromRequest(req.Duplicate),
		"etag":         etag,
		"fileName":     req.FileName,
		"parentFileId": parentID,
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
	if fileID != "" {
		if _, err := d.Delete(ctx, c, []drive.FileRef{{ID: fileID}}); err != nil {
			return nil, fmt.Errorf("pan123: 清理秒传未命中任务 %s: %w", fileID, err)
		}
	}
	return &drive.RapidUploadResult{Reuse: false, Message: "未命中秒传"}, nil
}

func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if method != "md5" {
		return "", nil
	}
	fid := toPan123FileID(fileID)
	if pooled, ok := poolGet(c, fid); ok && pooled.Etag != "" {
		return etagAsMd5(pooled.Etag), nil
	}
	detail, err := d.detail(ctx, c, fileID)
	if err != nil {
		return "", err
	}
	return etagAsMd5(detail.Etag), nil
}
