package pikpak

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

func (c *client) findUploadConflict(ctx context.Context, parentID, name string) (*File, error) {
	items, err := c.List(ctx, rootID(parentID), false)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if !items[i].Trashed && items[i].Name == name {
			return &items[i], nil
		}
	}
	return nil, nil
}

func pikpakConflictName(name string, index int) string {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s (%d)%s", stem, index, ext)
}

// prepareUploadTarget applies the shared conflict policy before hashing. This
// avoids expensive GCID work for refuse/skip and mirrors the legacy trash-based
// overwrite behavior.
func (d *Driver) prepareUploadTarget(ctx context.Context, cl *client, ui *model.UploadingUI) (bool, error) {
	name := strings.TrimSpace(ui.Info.Name)
	if name == "" {
		name = filepath.Base(ui.Info.LocalFilePath)
		ui.Info.Name = name
	}
	policy := driveutil.ResolveConflictPolicy(ui.Info.ConflictPolicy)
	for index := 1; ; index++ {
		conflict, err := cl.findUploadConflict(ctx, ui.Info.ParentFileID, name)
		if err != nil {
			return false, err
		}
		if conflict == nil {
			ui.Info.Name = name
			return true, nil
		}
		switch policy {
		case driveutil.ConflictRefuse:
			return false, fmt.Errorf("目标目录已存在同名文件：%s", conflict.Name)
		case driveutil.ConflictSkip:
			return false, nil
		case driveutil.ConflictRename:
			name = pikpakConflictName(name, index)
		default:
			if err := cl.Trash(ctx, []string{conflict.ID}); err != nil {
				return false, fmt.Errorf("处理同名文件失败：%w", err)
			}
			return true, nil
		}
	}
}

func markPikPakUploadComplete(ui *model.UploadingUI, fileID string) {
	ui.Upload.FileID = fileID
	ui.ReportUploadProgress(ui.Info.Size, ui.Info.Size)
	ui.Upload.IsDowning = false
	ui.Upload.IsCompleted = true
	ui.Upload.IsFailed = false
	ui.Upload.DownState = "completed"
	ui.Upload.FailedMessage = ""
}

func markPikPakUploadFailed(ui *model.UploadingUI, err error) {
	if ui == nil {
		return
	}
	ui.Upload.IsDowning = false
	ui.Upload.IsCompleted = false
	ui.Upload.IsFailed = true
	ui.Upload.DownState = "failed"
	if err != nil {
		ui.Upload.FailedMessage = err.Error()
	}
}

func cleanupPikPakUpload(cl *client, fileID string) error {
	if cl == nil || fileID == "" {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return cl.Delete(cleanupCtx, []string{fileID})
}

func normalizePikPakGCID(value string) (string, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 40 {
		return "", false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return value, true
}

func (d *Driver) prepareRapidUploadTarget(ctx context.Context, cl *client, req drive.RapidUploadRequest) (string, *drive.RapidUploadResult, error) {
	name := strings.TrimSpace(req.FileName)
	if name == "" {
		return "", nil, errors.New("pikpak: 文件名不能为空")
	}
	for index := 1; ; index++ {
		conflict, err := cl.findUploadConflict(ctx, req.ParentID, name)
		if err != nil {
			return "", nil, err
		}
		if conflict == nil {
			return name, nil, nil
		}
		switch req.Duplicate {
		case 1:
			return "", &drive.RapidUploadResult{Reuse: true, FileID: conflict.ID, ParentID: req.ParentID, Message: "目标文件已存在，已跳过"}, nil
		case 2:
			if err := cl.Trash(ctx, []string{conflict.ID}); err != nil {
				return "", nil, fmt.Errorf("pikpak: 处理同名文件失败: %w", err)
			}
			return name, nil, nil
		default:
			name = pikpakConflictName(req.FileName, index)
		}
	}
}

// RapidUploadByHash submits PikPak's native GCID before any source download.
// A miss creates a temporary upload entry, which is deleted before returning
// so the migration fallback cannot leave an orphan or rename around it.
func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if !strings.EqualFold(strings.TrimSpace(req.Method), "gcid") {
		return &drive.RapidUploadResult{Reuse: false, Message: "PikPak 仅支持 GCID 秒传"}, nil
	}
	gcid, ok := normalizePikPakGCID(req.Hash)
	if !ok {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 GCID 指纹"}, nil
	}
	if req.Size < 0 {
		return nil, errors.New("pikpak: 文件大小不能为负数")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	name, resolved, err := d.prepareRapidUploadTarget(ctx, cl, req)
	if err != nil || resolved != nil {
		return resolved, err
	}
	body := map[string]any{
		"kind": "drive#file", "name": name, "size": req.Size,
		"hash": gcid, "upload_type": "UPLOAD_TYPE_RESUMABLE",
		"resumable":   map[string]any{"provider": "PROVIDER_ALIYUN"},
		"folder_type": "NORMAL",
	}
	if parentID := apiParentID(req.ParentID); parentID != "" {
		body["parent_id"] = parentID
	}
	var response struct {
		UploadType string `json:"upload_type"`
		Resumable  *struct {
			Params *pikpakOSSParams `json:"params"`
		} `json:"resumable"`
		File *struct {
			ID    string `json:"id"`
			Phase string `json:"phase"`
		} `json:"file"`
		ID string `json:"id"`
	}
	if err := cl.jsonDo(ctx, httpMethodPost(), "/drive/v1/files", body, &response, nil); err != nil {
		return nil, err
	}
	fileID := response.ID
	if response.File != nil && response.File.ID != "" {
		fileID = response.File.ID
	}
	// rclone only treats PHASE_TYPE_COMPLETE as a GCID server-side hit.
	// A response without resumable parameters is otherwise ambiguous and must
	// not be reported as successful merely because it contains a file id.
	if response.File != nil && strings.EqualFold(strings.TrimSpace(response.File.Phase), "PHASE_TYPE_COMPLETE") {
		if fileID == "" {
			return nil, errors.New("pikpak: 秒传响应缺少 file id")
		}
		return &drive.RapidUploadResult{Reuse: true, FileID: fileID, ParentID: req.ParentID, Message: "秒传命中"}, nil
	}
	if response.Resumable == nil || response.Resumable.Params == nil {
		return nil, fmt.Errorf("pikpak: 秒传响应状态不明确 (upload_type=%q, phase=%q)", response.UploadType, func() string {
			if response.File == nil {
				return ""
			}
			return response.File.Phase
		}())
	}
	if fileID == "" {
		return nil, errors.New("pikpak: 秒传未命中且响应缺少待清理 file id")
	}
	if err := cl.Delete(ctx, []string{fileID}); err != nil {
		return nil, fmt.Errorf("pikpak: 秒传未命中，清理待上传对象失败: %w", err)
	}
	return &drive.RapidUploadResult{Reuse: false, ParentID: req.ParentID, Message: "未命中秒传"}, nil
}

// ResolveTransferHash reads the native GCID exposed in PikPak file metadata.
// It deliberately does not download the file merely to compute a hash; doing
// so would double source traffic when the target later reports a miss.
func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(method), "gcid") {
		return "", nil
	}
	cl, err := clientOf(c)
	if err != nil {
		return "", err
	}
	file, err := cl.Detail(ctx, fileID)
	if err != nil {
		return "", err
	}
	hash, ok := normalizePikPakGCID(file.Hash)
	if !ok {
		return "", nil
	}
	return strings.ToLower(hash), nil
}

// UploadOneFile uploads one file (GCID + create + optional OSS PUT).
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil || ui.Info.LocalFilePath == "" {
		return errors.New("pikpak: 上传文件路径为空")
	}
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	shouldUpload, err := d.prepareUploadTarget(ctx, cl, ui)
	if err != nil {
		markPikPakUploadFailed(ui, err)
		return err
	}
	if !shouldUpload {
		return nil
	}
	ui.Upload.DownState = "uploading"
	ui.Upload.IsDowning = true
	ui.Upload.IsFailed = false
	ui.Upload.IsCompleted = false
	gcid, err := computeGCID(ui.Info.LocalFilePath, ui)
	if err != nil {
		markPikPakUploadFailed(ui, err)
		return err
	}
	info, err := os.Stat(ui.Info.LocalFilePath)
	if err != nil {
		markPikPakUploadFailed(ui, err)
		return err
	}
	ui.Info.Size = info.Size()
	body := map[string]any{
		"kind": "drive#file", "name": ui.Info.Name, "size": ui.Info.Size,
		"hash": gcid, "upload_type": "UPLOAD_TYPE_RESUMABLE",
		"resumable":   map[string]any{"provider": "PROVIDER_ALIYUN"},
		"folder_type": "NORMAL",
	}
	if parentID := apiParentID(ui.Info.ParentFileID); parentID != "" {
		body["parent_id"] = parentID
	}
	var res struct {
		UploadType string `json:"upload_type"`
		Resumable  *struct {
			Params *pikpakOSSParams `json:"params"`
		} `json:"resumable"`
		File *struct {
			ID    string `json:"id"`
			Phase string `json:"phase"`
		} `json:"file"`
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	// Use a client where we can pass X-Captcha-Token if needed; basic flow:
	if err := cl.jsonDo(ctx, httpMethodPost(), "/drive/v1/files", body, &res, nil); err != nil {
		markPikPakUploadFailed(ui, err)
		return err
	}
	fileID := ""
	if res.File != nil {
		fileID = res.File.ID
	}
	if fileID == "" {
		fileID = res.ID
	}
	if fileID == "" {
		err := errors.New("pikpak: upload create missing file id")
		markPikPakUploadFailed(ui, err)
		return err
	}
	ui.Upload.FileID = fileID
	// Match rclone's completion rule: only an explicit COMPLETE phase means the
	// GCID was accepted server-side (zero-byte files use the same signal).
	if res.File != nil && strings.EqualFold(strings.TrimSpace(res.File.Phase), "PHASE_TYPE_COMPLETE") {
		markPikPakUploadComplete(ui, fileID)
		return nil
	}
	if res.Resumable == nil || res.Resumable.Params == nil {
		err := errors.New("pikpak: upload response has neither a completed file nor resumable parameters")
		cleanupErr := cleanupPikPakUpload(cl, fileID)
		markPikPakUploadFailed(ui, err)
		if cleanupErr != nil {
			return fmt.Errorf("%w；清理远端残留失败：%v", err, cleanupErr)
		}
		return err
	}
	params := res.Resumable.Params
	if err := ossPut(ctx, c, ui.Info.LocalFilePath, params, ui); err != nil {
		cleanupErr := cleanupPikPakUpload(cl, fileID)
		markPikPakUploadFailed(ui, err)
		if cleanupErr != nil {
			return fmt.Errorf("%w；清理远端残留失败：%v", err, cleanupErr)
		}
		return err
	}
	markPikPakUploadComplete(ui, fileID)
	return nil
}

func httpMethodPost() string { return "POST" }
