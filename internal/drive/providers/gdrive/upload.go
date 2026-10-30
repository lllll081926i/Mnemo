package gdrive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

const uploadChunkSize = 5 * 1024 * 1024

type uploadSession struct {
	FileID    string `json:"fileId"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	Overwrite bool   `json:"overwrite"`
}

func (*Driver) ManagesUploadAttempt() bool { return true }

func hashLocal(ctx context.Context, f *os.File, ui *model.UploadingUI, size int64) (string, error) {
	h := md5.New()
	buf := make([]byte, 256*1024)
	var done int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if ui.IsUploadStopRequested() {
			return "", context.Canceled
		}
		n, err := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
			done += int64(n)
			ui.ReportHashProgress(done, size)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (cl *client) reserveID(ctx context.Context) (string, error) {
	var response struct {
		IDs []string `json:"ids"`
	}
	if err := cl.json(ctx, http.MethodGet, "/files/generateIds", url.Values{"count": {"1"}, "space": {"drive"}, "type": {"files"}}, nil, &response); err != nil {
		return "", err
	}
	if len(response.IDs) != 1 || !validID(response.IDs[0]) {
		return "", errors.New("Google Drive 未返回预分配文件 ID")
	}
	return response.IDs[0], nil
}

func saveSession(key string, s uploadSession) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return drive.SaveUploadSessionState(key, string(data), nil)
}

func (cl *client) target(ctx context.Context, parent, name, policy string) (uploadSession, bool, error) {
	files, err := cl.listAll(ctx, quote(parent)+" in parents and trashed=false")
	if err != nil {
		return uploadSession{}, false, err
	}
	existing := []model.File{}
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
		if f.Name == name {
			existing = append(existing, f)
		}
	}
	s := uploadSession{Name: name}
	if len(existing) > 0 {
		switch driveutil.ResolveConflictPolicy(policy) {
		case driveutil.ConflictSkip:
			s.FileID = existing[0].FileID
			return s, true, nil
		case driveutil.ConflictRefuse:
			return s, false, errors.New("Google Drive 目标目录已存在同名项目")
		case driveutil.ConflictRename:
			ext := filepath.Ext(name)
			base := strings.TrimSuffix(name, ext)
			for i := 1; ; i++ {
				s.Name = fmt.Sprintf("%s (%d)%s", base, i, ext)
				if !names[s.Name] {
					break
				}
			}
		default:
			if len(existing) != 1 || existing[0].IsDir || strings.HasPrefix(existing[0].MimeType, "application/vnd.google-apps.") {
				return s, false, errors.New("Google Drive 同名目标不唯一或不是普通文件，不能安全覆盖")
			}
			s.FileID = existing[0].FileID
			s.Overwrite = true
		}
	}
	if s.FileID == "" {
		s.FileID, err = cl.reserveID(ctx)
	}
	return s, false, err
}

func (cl *client) startSession(ctx context.Context, s uploadSession, parent string, size int64) (string, error) {
	path := "/upload/drive/v3/files"
	method := http.MethodPost
	body := map[string]any{"id": s.FileID, "name": s.Name, "parents": []string{parent}}
	if s.Overwrite {
		path += "/" + s.FileID
		method = http.MethodPatch
		body = map[string]any{}
	}
	data, _ := json.Marshal(body)
	header := http.Header{"Content-Type": {"application/json; charset=UTF-8"}, "X-Upload-Content-Type": {"application/octet-stream"}, "X-Upload-Content-Length": {strconv.FormatInt(size, 10)}}
	status, h, response, err := cl.exchange(ctx, method, apiHost+path+"?uploadType=resumable&supportsAllDrives=true", data, header, false)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", statusError(status, response)
	}
	sessionURL := h.Get("Location")
	if !trustedGoogleURL(sessionURL) {
		return "", errors.New("Google Drive 未返回有效上传会话地址，已保留目标文件 ID 以供核对")
	}
	return sessionURL, nil
}

func (cl *client) confirmUpload(ctx context.Context, s uploadSession, parent, hash string, size int64) (resultErr error) {
	cl.confirming = true
	defer func() {
		cl.confirming = false
		if cl.credentialError != nil {
			if resultErr == nil {
				drive.PersistCompletedUploadToken(cl.c)
			} else {
				resultErr = errors.Join(resultErr, cl.credentialError)
			}
			cl.credentialError = nil
		}
	}()
	f, err := cl.detail(ctx, s.FileID)
	if err != nil {
		return err
	}
	if f.ID != s.FileID || f.Size == nil || *f.Size != size || f.Trashed || f.MD5 == "" || !strings.EqualFold(f.MD5, hash) || f.Name != s.Name || f.MIME == folderMIME || f.Shortcut != nil {
		return errors.New("Google Drive 目标文件大小、名称或校验和未匹配，上传结果尚未确认")
	}
	for _, p := range f.Parents {
		if p == parent {
			return nil
		}
	}
	return errors.New("Google Drive 上传目标不在预期目录，上传结果尚未确认")
}

func sessionOffset(h http.Header, size int64) (int64, error) {
	raw := h.Get("Range")
	if raw == "" {
		return 0, nil
	}
	if !strings.HasPrefix(raw, "bytes=0-") {
		return 0, errors.New("Google Drive 上传回执的 Range 无效")
	}
	n, err := number(strings.TrimPrefix(raw, "bytes=0-"))
	if err != nil || n >= size {
		return 0, errors.New("Google Drive 上传回执超出文件大小")
	}
	return n + 1, nil
}

func (cl *client) querySession(ctx context.Context, s uploadSession, size int64) (int64, bool, error) {
	h := http.Header{"Content-Range": {"bytes */" + strconv.FormatInt(size, 10)}, "Content-Type": {"application/octet-stream"}}
	status, header, data, err := cl.exchange(ctx, http.MethodPut, s.URL, nil, h, false)
	if err != nil {
		return 0, false, err
	}
	if status == 200 || status == 201 {
		return size, true, nil
	}
	if status != 308 {
		return 0, false, statusError(status, data)
	}
	offset, err := sessionOffset(header, size)
	return offset, false, err
}

func waitRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(1<<attempt) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (cl *client) sendChunks(ctx context.Context, s uploadSession, f *os.File, ui *model.UploadingUI, size, offset int64) error {
	buf := make([]byte, uploadChunkSize)
	stalls, recoveries := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ui.IsUploadStopRequested() {
			return context.Canceled
		}
		length := min(int64(len(buf)), size-offset)
		if length < 0 {
			return errors.New("Google Drive 续传偏移超出文件大小")
		}
		if length == 0 && size > 0 {
			return errors.New("Google Drive 已接收全部字节但尚未确认文件提交，请核对目标文件")
		}
		chunk := buf[:length]
		if _, err := io.ReadFull(io.NewSectionReader(f, offset, length), chunk); err != nil {
			return err
		}
		rangeValue := fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, size)
		if size == 0 {
			rangeValue = "bytes */0"
		}
		h := http.Header{"Content-Range": {rangeValue}, "Content-Type": {"application/octet-stream"}}
		status, header, data, err := cl.exchange(ctx, http.MethodPut, s.URL, chunk, h, true)
		if err == nil && (status == 200 || status == 201) {
			return nil
		}
		if err == nil && status == 308 {
			next, parseErr := sessionOffset(header, size)
			if parseErr != nil {
				return parseErr
			}
			if next < offset || next > offset+length {
				return errors.New("Google Drive 回执偏移与发送范围不一致")
			}
			if next == offset {
				stalls++
			} else {
				stalls = 0
			}
			if stalls > 3 {
				return errors.New("Google Drive 上传未推进，已保留会话")
			}
			offset = next
			ui.ReportUploadProgress(min(offset, max(0, size-1)), size)
			continue
		}
		if err == nil && status != 429 && status < 500 {
			return statusError(status, data)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if recoveries >= 3 {
			return errors.New("Google Drive 分片恢复次数已达上限，会话已保留")
		}
		if err := waitRetry(ctx, recoveries); err != nil {
			return err
		}
		recoveries++
		next, done, queryErr := cl.querySession(ctx, s, size)
		if queryErr != nil {
			return fmt.Errorf("Google Drive 上传响应丢失且会话状态无法确认：%w", queryErr)
		}
		if done {
			return nil
		}
		if next < offset || next > offset+length {
			return errors.New("Google Drive 会话恢复偏移与发送范围不一致")
		}
		offset = next
	}
}

func (*Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil || strings.TrimSpace(ui.Info.LocalFilePath) == "" || strings.TrimSpace(ui.Info.Name) == "" {
		return errors.New("Google Drive 上传文件路径或名称为空")
	}
	cl, err := newClient(c)
	if err != nil {
		return err
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
	if !info.Mode().IsRegular() {
		return errors.New("Google Drive 上传来源必须是普通文件")
	}
	ui.Info.Size = info.Size()
	parent, err := cl.folder(ctx, ui.Info.ParentFileID)
	if err != nil {
		return err
	}
	hash, err := hashLocal(ctx, f, ui, info.Size())
	if err != nil {
		return err
	}
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return errors.New("上传来源在哈希计算期间发生变化，请重新添加任务")
	}
	key := drive.UploadSessionKey(c.UserID, c.DriveID, parent, ui.UploadID+"\x00"+ui.Info.LocalFilePath+"\x00"+ui.Info.ConflictPolicy+"\x00"+hash+"\x00"+strconv.FormatInt(info.ModTime().UnixNano(), 10), info.Size())
	raw, _ := drive.LoadUploadSessionState(key)
	var s uploadSession
	if raw != "" {
		if json.Unmarshal([]byte(raw), &s) != nil || !validID(s.FileID) || s.Name == "" || s.URL != "" && !trustedGoogleURL(s.URL) {
			return errors.New("Google Drive 上传检查点无效，不能自动重新提交")
		}
	} else {
		var skipped bool
		s, skipped, err = cl.target(ctx, parent, ui.Info.Name, ui.Info.ConflictPolicy)
		if err != nil {
			return err
		}
		if skipped {
			ui.Upload.FileID = s.FileID
			return nil
		}
		if err := saveSession(key, s); err != nil {
			return err
		}
	}
	ui.Upload.FileID = s.FileID
	ui.Info.Name = s.Name
	if err := ui.BeginUploadAttempt(); err != nil {
		return err
	}
	if raw != "" && cl.confirmUpload(ctx, s, parent, hash, info.Size()) == nil {
		drive.ClearUploadSession(key)
		ui.ReportUploadProgress(info.Size(), info.Size())
		return nil
	}
	offset := int64(0)
	if s.URL != "" {
		var done bool
		offset, done, err = cl.querySession(ctx, s, info.Size())
		if err != nil {
			if !isStatus(err, 404) && !isStatus(err, 410) {
				return err
			}
			remote, lookupErr := cl.detail(ctx, s.FileID)
			if s.Overwrite {
				if lookupErr != nil || remote.Name != s.Name || remote.Trashed || remote.MIME == folderMIME || remote.Shortcut != nil || len(remote.Parents) != 1 || remote.Parents[0] != parent {
					return errors.New("Google Drive 覆盖目标状态发生变化，不能重新建立上传会话")
				}
			} else if !isStatus(lookupErr, 404) {
				return errors.New("Google Drive 会话已失效，远端目标状态尚未确认，不能创建第二个文件")
			}
			s.URL = ""
		} else if done {
			if err := cl.confirmUpload(ctx, s, parent, hash, info.Size()); err != nil {
				return err
			}
			drive.ClearUploadSession(key)
			ui.ReportUploadProgress(info.Size(), info.Size())
			return nil
		}
	}
	if s.URL == "" {
		s.URL, err = cl.startSession(ctx, s, parent, info.Size())
		if err != nil {
			if isStatus(err, 409) && cl.confirmUpload(ctx, s, parent, hash, info.Size()) == nil {
				drive.ClearUploadSession(key)
				ui.ReportUploadProgress(info.Size(), info.Size())
				return nil
			}
			return err
		}
		if err := saveSession(key, s); err != nil {
			return err
		}
	}
	ui.ReportUploadStage("uploading")
	if err := cl.sendChunks(ctx, s, f, ui, info.Size(), offset); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cl.confirmUpload(ctx, s, parent, hash, info.Size()) != nil {
			return err
		}
	}
	if err := cl.confirmUpload(ctx, s, parent, hash, info.Size()); err != nil {
		return err
	}
	drive.ClearUploadSession(key)
	ui.ReportUploadProgress(info.Size(), info.Size())
	return nil
}
