package onedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// Graph simple upload limit (4 MiB).
const smallUploadLimit = 4 * 1024 * 1024

// Upload session chunk size: multiple of 320 KiB per Graph rules.
const sessionChunkSize = 10 * 1024 * 1024

func encodePathSegment(v string) string {
	return url.PathEscape(v)
}

// smallUploadPath builds the PUT content path for small files.
func smallUploadPath(parentID, fileName string) string {
	name := encodePathSegment(fileName)
	if parentID == "" || parentID == RootID {
		return "/me/drive/root:/" + name + ":/content"
	}
	return "/me/drive/items/" + url.PathEscape(parentID) + ":/" + name + ":/content"
}

// rawPut performs a PUT with an auth header.
func (c *client) rawPut(ctx context.Context, target string, body io.Reader) (string, error) {
	resp, err := c.http.DoUpload(ctx, http.MethodPut, target, c.headers(map[string]string{"Content-Type": "application/octet-stream"}), body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", graphError(data, resp.StatusCode)
	}
	var item struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(data, &item)
	if strings.TrimSpace(item.ID) == "" {
		return "", errors.New("onedrive: upload response missing file id")
	}
	return item.ID, nil
}

// sessionUpload uploads a file via a Graph upload session with chunked PUTs,
// reporting progress through the upload UI model.
func (c *client) sessionUpload(ctx context.Context, dc drive.Context, f *os.File, parentID, name string, ui *model.UploadingUI, conflictBehavior string) error {
	if ui == nil {
		return errors.New("onedrive: 上传任务为空")
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	identity := strings.TrimSpace(ui.Info.SHA1)
	if identity == "" {
		identity = fmt.Sprintf("mtime:%d", info.ModTime().UnixNano())
	} else {
		identity += fmt.Sprintf("\x00mtime:%d", info.ModTime().UnixNano())
	}
	// Include content identity and conflict behavior so a same-size replacement
	// or a changed policy cannot resume an unrelated Graph session.
	sessionName := name + "\x00" + conflictBehavior + "\x00" + identity
	sessionKey := drive.UploadSessionKey(dc.UserID, dc.DriveID, parentID, sessionName, size)
	// Reuse the remote session when its URL is still alive. A byte offset alone
	// is not enough because a newly-created Graph session has no uploaded data.
	savedSessionID, _ := drive.LoadUploadSessionState(sessionKey)
	var sess *UploadSessionItem
	var pos int64
	if savedSessionID != "" {
		candidate := &UploadSessionItem{UploadURL: savedSessionID}
		if qPos, qFile, completed, usable, status := querySessionPosition(ctx, candidate.UploadURL, c); usable {
			sess, pos = candidate, qPos
			if completed {
				if ui != nil {
					ui.Upload.FileID = qFile
				}
				drive.ClearUploadSession(sessionKey)
				return nil
			}
		} else if status != http.StatusNotFound {
			return fmt.Errorf("onedrive: 上传会话状态暂不明确（HTTP %d），保留会话等待重试", status)
		}
	}
	if sess == nil {
		sess, err = c.CreateUploadSession(ctx, parentID, name, conflictBehavior)
		if err != nil {
			return err
		}
	}
	if sess == nil || strings.TrimSpace(sess.UploadURL) == "" {
		return errors.New("onedrive: upload session url missing")
	}
	if err := drive.SaveUploadSessionState(sessionKey, sess.UploadURL, []int{int(pos)}); err != nil {
		return err
	}

	// Query a newly-created session too; this keeps the handling consistent
	// when the provider returns an already-initialized session.
	if pos == 0 {
		if qPos, qFile, completed, usable, _ := querySessionPosition(ctx, sess.UploadURL, c); usable {
			if completed {
				if ui != nil {
					ui.Upload.FileID = qFile
				}
				drive.ClearUploadSession(sessionKey)
				return nil
			}
			if qPos > 0 && qPos <= size {
				pos = qPos
			}
		}
	}

	buf := make([]byte, sessionChunkSize)
	retries := make(map[int64]int)
	recoveryCount := 0
	for pos < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		length := min(int64(len(buf)), size-pos)
		n, err := io.ReadFull(io.NewSectionReader(f, pos, length), buf[:length])
		if err != nil {
			return err
		}
		chunk := buf[:n]
		end := pos + int64(n) - 1
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, sess.UploadURL, bytes.NewReader(chunk))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Range", "bytes "+strconv.FormatInt(pos, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
		req.Header.Set("Content-Length", strconv.Itoa(n))
		resp, err := netx.DoUpload(c.http.HTTP, req)
		var body []byte
		var readErr error
		if resp != nil {
			body, readErr = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
		}
		if err != nil || readErr != nil || resp != nil && (resp.StatusCode == 416 || resp.StatusCode == 429 || resp.StatusCode >= 500) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if retries[pos] >= 3 || recoveryCount >= 12 {
				return errors.New("onedrive: 上传分片恢复次数已达上限，会话已保留")
			}
			delay := time.Second << retries[pos]
			retries[pos]++
			recoveryCount++
			if resp != nil && resp.Header.Get("Retry-After") != "" {
				if err := waitUploadRetry(ctx, resp.Header.Get("Retry-After"), delay); err != nil {
					return err
				}
			}
			qPos, qFile, completed, usable, _ := querySessionPosition(ctx, sess.UploadURL, c)
			if completed && usable {
				ui.Upload.FileID = qFile
				ui.ReportUploadProgress(size, size)
				drive.ClearUploadSession(sessionKey)
				return nil
			}
			if !usable || qPos < 0 || qPos > size {
				return errors.New("onedrive: 远端回执不明且无法查询会话，保留检查点，不重复提交")
			}
			if qPos == pos && (resp == nil || resp.Header.Get("Retry-After") == "") {
				if err := waitUploadRetry(ctx, "", delay); err != nil {
					return err
				}
			}
			pos = qPos
			if err := drive.SaveUploadSessionState(sessionKey, sess.UploadURL, []int{int(pos)}); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode >= 300 && resp.StatusCode != 201 && resp.StatusCode != 202 && resp.StatusCode != 200 {
			return graphError(body, resp.StatusCode)
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			var item struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(body, &item) != nil || strings.TrimSpace(item.ID) == "" {
				return errors.New("onedrive: 上传提交未返回文件 ID")
			}
			if pos+int64(n) != size {
				return errors.New("onedrive: 文件未传完但服务端返回已完成")
			}
			ui.Upload.FileID = item.ID
			ui.ReportUploadProgress(size, size)
			drive.ClearUploadSession(sessionKey)
			return nil
		}
		var pending struct {
			NextExpectedRanges []string `json:"nextExpectedRanges"`
		}
		if json.Unmarshal(body, &pending) != nil || len(pending.NextExpectedRanges) == 0 {
			return errors.New("onedrive: 分片回执缺少续传位置，会话已保留")
		}
		next, err := strconv.ParseInt(strings.SplitN(pending.NextExpectedRanges[0], "-", 2)[0], 10, 64)
		if err != nil || next <= pos || next > pos+int64(n) || next > size {
			return errors.New("onedrive: 分片回执位置异常，会话已保留")
		}
		pos = next
		// Persist progress incrementally (byte offset as single-element part list)
		if err := drive.SaveUploadSessionState(sessionKey, sess.UploadURL, []int{int(pos)}); err != nil {
			return fmt.Errorf("onedrive: 保存上传续传进度失败: %w", err)
		}
		if ui != nil {
			ui.ReportUploadProgress(pos, size)
		}
	}
	if _, fileID, completed, usable, _ := querySessionPosition(ctx, sess.UploadURL, c); usable && completed {
		ui.Upload.FileID = fileID
		drive.ClearUploadSession(sessionKey)
		return nil
	}
	return errors.New("onedrive: 分片已发送，但服务端尚未确认上传完成")
}

// querySessionPosition queries the Graph upload session URL to discover the
// next expected byte range (resumable upload). Returns position, fileId (when
// the session is already complete), and a completed flag.
func querySessionPosition(ctx context.Context, uploadURL string, c *client) (pos int64, fileID string, completed, usable bool, status int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uploadURL, nil)
	if err != nil {
		return 0, "", false, false, 0
	}
	resp, err := c.http.HTTP.Do(req)
	if err != nil {
		return 0, "", false, false, 0
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return 0, "", false, false, status
	}
	// 200/201/202 with a file id = session complete
	if resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 202 {
		var done struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &done) == nil && done.ID != "" {
			return 0, done.ID, true, true, status
		}
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return 0, "", false, false, status
	}
	var data struct {
		NextExpectedRanges []string `json:"nextExpectedRanges"`
	}
	if json.Unmarshal(body, &data) != nil {
		return 0, "", false, false, status
	}
	if len(data.NextExpectedRanges) == 0 {
		return 0, "", false, false, status
	}
	first := data.NextExpectedRanges[0]
	if idx := indexByte(first, '-'); idx > 0 {
		first = first[:idx]
	}
	position, err := strconv.ParseInt(first, 10, 64)
	if err != nil || position < 0 {
		return 0, "", false, false, status
	}
	return position, "", false, true, status
}

func waitUploadRetry(ctx context.Context, retryAfter string, fallback time.Duration) error {
	delay := fallback
	if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil && seconds >= 0 {
		if seconds > 300 {
			return errors.New("onedrive: 服务端要求长时间冷却，请稍后恢复任务")
		}
		delay = time.Duration(seconds) * time.Second
	} else if until, err := http.ParseTime(retryAfter); err == nil {
		delay = max(time.Duration(0), time.Until(until))
	}
	if delay > 5*time.Minute {
		return errors.New("onedrive: 服务端要求长时间冷却，请稍后恢复任务")
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

var _ = json.Marshal
var _ = fmt.Sprint
var _ = errors.New
