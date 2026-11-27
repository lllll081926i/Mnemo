package pan123

import (
	"bytes"
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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	maxChunk                = 16 * 1024 * 1024 // 单片 ≤16MB（AList newUpload）
	pan123UploadConcurrency = 3                // 并发分片上限，兼顾提速与内存占用
	pan123PersistEveryParts = 3                // 批量持久化已完成分片，减少磁盘 JSON 写放大
)

var (
	errPan123UploadSessionInvalid = errors.New("123: 上传会话已失效")
	errPan123UploadStopped        = errors.New("123: 上传已停止")
)

// ChunkPlan is the 123 upload chunk plan.
type ChunkPlan struct {
	ChunkSize  int64
	ChunkCount int64
}

// calcPan123ChunkPlan mirrors legacy calcPan123ChunkPlan: single chunk ≤16MB;
// a 0-byte file degrades to one empty chunk (chunkSize=1 guard).
func calcPan123ChunkPlan(size int64) ChunkPlan {
	chunkSize := size
	if chunkSize > maxChunk {
		chunkSize = maxChunk
	}
	if chunkSize <= 0 {
		chunkSize = 1
	}
	chunkCount := size / chunkSize
	if size%chunkSize != 0 {
		chunkCount++
	}
	if chunkCount < 1 {
		chunkCount = 1
	}
	return ChunkPlan{ChunkSize: chunkSize, ChunkCount: chunkCount}
}

// UploadRequestData is the /file/upload_request response payload.
type UploadRequestData struct {
	AccessKeyID     string `json:"AccessKeyId"`
	Bucket          string `json:"Bucket"`
	Key             string `json:"Key"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken"`
	FileID          string `json:"FileId"`
	Reuse           bool   `json:"Reuse"`
	EndPoint        string `json:"EndPoint"`
	StorageNode     string `json:"StorageNode"`
	UploadID        string `json:"UploadId"`
}

// pan123UploadSession contains only the fields needed to resume an upload.
// Temporary access keys from upload_request are intentionally not persisted.
type pan123UploadSession struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	FileID      string `json:"fileId"`
	StorageNode string `json:"storageNode"`
	UploadID    string `json:"uploadId"`
}

func encodePan123UploadSession(data UploadRequestData) string {
	b, _ := json.Marshal(pan123UploadSession{
		Bucket:      data.Bucket,
		Key:         data.Key,
		FileID:      data.FileID,
		StorageNode: data.StorageNode,
		UploadID:    data.UploadID,
	})
	return string(b)
}

func decodePan123UploadSession(raw string) (UploadRequestData, bool) {
	var session pan123UploadSession
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &session) != nil {
		return UploadRequestData{}, false
	}
	if strings.TrimSpace(session.Bucket) == "" || strings.TrimSpace(session.Key) == "" ||
		strings.TrimSpace(session.FileID) == "" || strings.TrimSpace(session.StorageNode) == "" ||
		strings.TrimSpace(session.UploadID) == "" {
		return UploadRequestData{}, false
	}
	return UploadRequestData{
		Bucket:      session.Bucket,
		Key:         session.Key,
		FileID:      session.FileID,
		StorageNode: session.StorageNode,
		UploadID:    session.UploadID,
	}, true
}

func pan123UploadSessionKey(c drive.Context, parentID, name string, size int64, fileMD5 string) string {
	identity := name + "\x00md5:" + strings.ToLower(strings.TrimSpace(fileMD5))
	return drive.UploadSessionKey(c.UserID, c.DriveID, parentID, identity, size)
}

func parseUploadRequestData(raw map[string]any) UploadRequestData {
	return UploadRequestData{
		AccessKeyID:     firstString(raw, "AccessKeyId", "accessKeyId"),
		Bucket:          firstString(raw, "Bucket", "bucket"),
		Key:             firstString(raw, "Key", "key"),
		SecretAccessKey: firstString(raw, "SecretAccessKey", "secretAccessKey"),
		SessionToken:    firstString(raw, "SessionToken", "sessionToken"),
		FileID:          firstString(raw, "FileId", "fileId"),
		Reuse:           asBool(pick(raw, "Reuse", "reuse")),
		EndPoint:        firstString(raw, "EndPoint", "endPoint"),
		StorageNode:     firstString(raw, "StorageNode", "storageNode"),
		UploadID:        firstString(raw, "UploadId", "uploadId"),
	}
}

// fileMD5 computes the lowercase hex MD5 of the whole local file.
// fileMD5 computes the MD5 hash while reporting progress via ui.Upload.
func fileMD5(ctx context.Context, path string, ui *model.UploadingUI) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	buf := make([]byte, 256*1024)
	var read int64
	for {
		if stopped(ctx, ui) {
			return "", context.Canceled
		}
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			read += int64(n)
			if ui != nil {
				ui.ReportHashProgress(read, ui.Info.Size)
			}
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

// presignBatch fetches PUT urls for part numbers [start, end).
func (d *Driver) presignBatch(ctx context.Context, c drive.Context, api string, data UploadRequestData, start, end int64) (map[string]string, error) {
	resp, err := d.api(ctx, c, http.MethodPost, api, map[string]any{
		"bucket":          data.Bucket,
		"key":             data.Key,
		"partNumberEnd":   end,
		"partNumberStart": start,
		"uploadId":        data.UploadID,
		"StorageNode":     data.StorageNode,
	}, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		var apiErr *pan123APIError
		if errors.As(err, &apiErr) && apiErr.Code != 401 && apiErr.Code != 403 {
			return nil, fmt.Errorf("%w: %v", errPan123UploadSessionInvalid, err)
		}
		return nil, err
	}
	dm := parseMap(resp.Data)
	m := map[string]string{}
	for _, key := range []string{"presignedUrls", "PreSignedUrls"} {
		if raw, ok := dm[key].(map[string]any); ok {
			for k, v := range raw {
				m[k] = asString(v)
			}
		}
	}
	return m, nil
}

// putChunk PUTs one chunk body to a presigned url; returns the http status.
func putChunk(ctx context.Context, rawURL string, body []byte) (int, error) {
	hc := netx.NewClient(5 * time.Minute)
	resp, err := hc.DoUpload(ctx, http.MethodPut, rawURL, map[string]string{
		"Content-Type": "application/octet-stream",
		"User-Agent":   ua,
	}, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	// Return the status even for HTTP errors. The caller must be able to
	// distinguish an expired presigned URL (403) and refresh it before failing.
	return resp.StatusCode, nil
}

// readSlice reads exactly n bytes at offset (mirrors readProviderUploadSlice).
func readSlice(f *os.File, offset, n int64) ([]byte, error) {
	if n <= 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	m, err := io.ReadFull(io.NewSectionReader(f, offset, n), buf)
	if err != nil {
		return nil, err
	}
	return buf[:m], nil
}

func (d *Driver) uploadPan123Parts(ctx context.Context, c drive.Context, ui *model.UploadingUI, data UploadRequestData, sessionKey string, savedParts []int, f *os.File, size int64) error {
	plan := calcPan123ChunkPlan(size)
	batchSize := int64(1)
	if plan.ChunkCount > 1 {
		batchSize = 10 // 多片时一次最多预签 10 个 URL
	}
	api := apiS3Auth
	if plan.ChunkCount > 1 {
		api = apiS3Prepare
	}

	var stateMu sync.Mutex
	uploadedSet := make(map[int]bool, len(savedParts))
	var uploadedBytes atomic.Int64
	for _, pn := range savedParts {
		if pn >= 1 && int64(pn) <= plan.ChunkCount {
			uploadedSet[pn] = true
			offset := int64(pn-1) * plan.ChunkSize
			cur := plan.ChunkSize
			if size-offset < cur {
				cur = size - offset
			}
			uploadedBytes.Add(cur)
		}
	}
	if uploadedBytes.Load() > 0 {
		ui.ReportUploadProgress(uploadedBytes.Load(), size)
	}

	encodedSession := encodePan123UploadSession(data)
	pendingPersists := 0
	flushState := func(force bool) error {
		stateMu.Lock()
		if !force && pendingPersists < pan123PersistEveryParts {
			stateMu.Unlock()
			return nil
		}
		parts := drive.SortedUniqueParts(uploadedSet)
		pendingPersists = 0
		stateMu.Unlock()
		if err := drive.SaveUploadSessionState(sessionKey, encodedSession, parts); err != nil {
			return fmt.Errorf("123: 保存上传分片进度失败: %w", err)
		}
		return nil
	}

	for i := int64(1); i <= plan.ChunkCount; i += batchSize {
		if stopped(ctx, ui) {
			return errPan123UploadStopped
		}
		start := i
		end := i + batchSize
		if end > plan.ChunkCount+1 {
			end = plan.ChunkCount + 1
		}

		// 预签名校验（同时用于验证续传的 uploadId 在远端仍有效）
		urls, err := d.presignBatch(ctx, c, api, data, start, end)
		if err != nil {
			return err
		}

		pending := make([]int64, 0, end-start)
		stateMu.Lock()
		for j := start; j < end; j++ {
			if !uploadedSet[int(j)] {
				pending = append(pending, j)
			}
		}
		stateMu.Unlock()
		if len(pending) == 0 {
			continue
		}

		var urlMu sync.RWMutex
		var presignEpoch uint64
		getPartURL := func(part int64) (string, uint64) {
			urlMu.RLock()
			defer urlMu.RUnlock()
			return urls[strconv.FormatInt(part, 10)], presignEpoch
		}
		refreshBatchURLs := func(seenEpoch uint64, part int64) (string, error) {
			urlMu.Lock()
			defer urlMu.Unlock()
			if presignEpoch == seenEpoch {
				nextURLs, err := d.presignBatch(ctx, c, api, data, start, end)
				if err != nil {
					return "", err
				}
				urls = nextURLs
				presignEpoch++
			}
			return urls[strconv.FormatInt(part, 10)], nil
		}

		batchCtx, cancelBatch := context.WithCancel(ctx)
		workers := pan123UploadConcurrency
		if len(pending) < workers {
			workers = len(pending)
		}
		if workers < 1 {
			workers = 1
		}
		var nextIdx atomic.Int64
		var errMu sync.Mutex
		var firstErr error
		setErr := func(e error) {
			if e == nil {
				return
			}
			errMu.Lock()
			if firstErr == nil {
				firstErr = e
				cancelBatch()
			}
			errMu.Unlock()
		}

		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					if stopped(batchCtx, ui) {
						setErr(errPan123UploadStopped)
						return
					}
					idx := int(nextIdx.Add(1) - 1)
					if idx >= len(pending) {
						return
					}
					j := pending[idx]
					offset := (j - 1) * plan.ChunkSize
					cur := plan.ChunkSize
					if size-offset < cur {
						cur = size - offset
					}
					uploadURL, epoch := getPartURL(j)
					if uploadURL == "" {
						setErr(fmt.Errorf("%w: 获取分片 %d 上传地址失败", errPan123UploadSessionInvalid, j))
						return
					}
					buff, readErr := readSlice(f, offset, cur)
					if readErr != nil {
						setErr(fmt.Errorf("读取上传文件失败: %w", readErr))
						return
					}
					status, putErr := putChunk(batchCtx, uploadURL, buff)
					if putErr != nil {
						if stopped(ctx, ui) {
							setErr(errPan123UploadStopped)
						} else {
							setErr(putErr)
						}
						return
					}
					if status == http.StatusForbidden {
						retryURL, rerr := refreshBatchURLs(epoch, j)
						if rerr != nil {
							setErr(rerr)
							return
						}
						if retryURL == "" {
							setErr(fmt.Errorf("%w: 获取分片 %d 重试上传地址失败", errPan123UploadSessionInvalid, j))
							return
						}
						status, putErr = putChunk(batchCtx, retryURL, buff)
						if putErr != nil {
							setErr(putErr)
							return
						}
					}
					if status == http.StatusForbidden {
						drive.ClearUploadSession(sessionKey)
						setErr(fmt.Errorf("%w: 分片上传失败 HTTP %d", errPan123UploadSessionInvalid, status))
						return
					}
					if status < http.StatusOK || status >= http.StatusMultipleChoices {
						setErr(fmt.Errorf("分片上传失败 HTTP %d", status))
						return
					}
					stateMu.Lock()
					uploadedSet[int(j)] = true
					pendingPersists++
					stateMu.Unlock()
					if saveErr := flushState(false); saveErr != nil {
						setErr(saveErr)
						return
					}
					ui.ReportUploadProgress(uploadedBytes.Add(cur), size)
				}
			}()
		}
		wg.Wait()
		cancelBatch()

		if firstErr != nil {
			// 出错或暂停时保留已完成的断点，避免重传已完成分片
			if !errors.Is(firstErr, errPan123UploadSessionInvalid) {
				_ = flushState(true)
			}
			return firstErr
		}
		if err := flushState(true); err != nil {
			return err
		}
	}
	return nil
}

// UploadOneFile uploads a single file: MD5 → upload_request (可能秒传) →
// 分片预签名 PUT → upload_complete。
func (d *Driver) ManagesUploadAttempt() bool { return true }

func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil {
		return errors.New("123: 无效的上传任务")
	}
	mark := func(done bool, failed bool, msg string) {
		if done {
			ui.Upload.IsDowning = false
			ui.Upload.IsCompleted = true
			ui.Upload.DownState = "completed"
		} else if failed {
			ui.Upload.IsDowning = false
			ui.Upload.IsFailed = true
			ui.Upload.DownState = "failed"
			ui.Upload.FailedMessage = msg
		}
	}

	etag, err := fileMD5(ctx, ui.Info.LocalFilePath, ui)
	if err != nil {
		mark(false, true, "计算 MD5 失败: "+err.Error())
		return err
	}
	if etag == "" {
		mark(false, true, "计算 MD5 失败")
		return errors.New("计算 MD5 失败")
	}
	ui.ReportUploadStage("checking")
	if stopped(ctx, ui) {
		markStopped(ui)
		return ctx.Err()
	}

	stat, err := os.Stat(ui.Info.LocalFilePath)
	if err != nil {
		mark(false, true, "获取上传文件信息失败: "+err.Error())
		return err
	}
	size := stat.Size()
	ui.Info.Size = size
	parentFileID := toPan123Number(ui.Info.ParentFileID)
	sessionKey := pan123UploadSessionKey(c, strconv.FormatInt(parentFileID, 10), ui.Info.Name, size, etag)
	savedSessionID, savedParts := drive.LoadUploadSessionState(sessionKey)
	data, resumed := decodePan123UploadSession(savedSessionID)

	ui.Upload.DownState = "uploading"
	ui.Upload.IsDowning = true

	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		mark(false, true, "打开文件失败: "+err.Error())
		return err
	}
	defer f.Close()

	plan := calcPan123ChunkPlan(size)
	if err := ui.BeginUploadAttempt(); err != nil {
		return err
	}
	for sessionAttempt := 0; sessionAttempt < 2; sessionAttempt++ {
		if !resumed {
			reqResp, reqErr := d.api(ctx, c, http.MethodPost, apiUploadReq, map[string]any{
				"driveId":      0,
				"duplicate":    duplicateFromPolicy(ui.Info.ConflictPolicy),
				"etag":         etag,
				"fileName":     ui.Info.Name,
				"parentFileId": parentFileID,
				"size":         size,
				"type":         0,
			}, nil)
			if reqErr != nil {
				mark(false, true, reqErr.Error())
				return reqErr
			}
			data = parseUploadRequestData(parseMap(reqResp.Data))
			if strings.TrimSpace(data.FileID) == "" || data.FileID == "0" {
				err := errors.New("123: 上传初始化未返回有效 fileId")
				mark(false, true, err.Error())
				return err
			}
			if data.Reuse {
				// 本地 MD5 命中秒传
				ui.Info.IsMiaoChuan = true
				ui.Upload.FileID = data.FileID
				ui.ReportUploadProgress(size, size)
				drive.ClearUploadSession(sessionKey)
				mark(true, false, "")
				return nil
			}
			if strings.TrimSpace(data.UploadID) == "" {
				err := errors.New("123: 上传初始化未返回 uploadId")
				mark(false, true, err.Error())
				return err
			}
			// Persist the remote session before the first part so a stop or process
			// exit can resume even when no part has completed yet.
			ui.Upload.FileID = data.FileID
			if err := drive.SaveUploadSessionState(sessionKey, encodePan123UploadSession(data), nil); err != nil {
				return fmt.Errorf("123: 保存上传会话失败: %w", err)
			}
		}
		ui.Upload.UploadID = data.UploadID
		ui.Upload.FileID = data.FileID

		uploadErr := d.uploadPan123Parts(ctx, c, ui, data, sessionKey, savedParts, f, size)
		if errors.Is(uploadErr, errPan123UploadStopped) {
			markStopped(ui)
			return ctx.Err()
		}
		if uploadErr != nil {
			if resumed && sessionAttempt == 0 && errors.Is(uploadErr, errPan123UploadSessionInvalid) {
				// The provider rejected the saved remote session. Drop only this
				// session and start one fresh upload_request; network/auth errors
				// never take this path and retain resumable state.
				drive.ClearUploadSession(sessionKey)
				savedParts = nil
				data = UploadRequestData{}
				resumed = false
				ui.ReportUploadProgress(0, size)
				continue
			}
			mark(false, true, uploadErr.Error())
			return uploadErr
		}

		completeBody := map[string]any{
			"StorageNode": data.StorageNode,
			"bucket":      data.Bucket,
			"fileId":      data.FileID,
			"fileSize":    size,
			"isMultipart": plan.ChunkCount > 1,
			"key":         data.Key,
			"uploadId":    data.UploadID,
		}
		_, err = d.api(ctx, c, http.MethodPost, apiUploadDoneV2, completeBody, nil)
		if err != nil {
			// 回退到 v1
			if _, err2 := d.api(ctx, c, http.MethodPost, apiUploadDone, map[string]any{
				"fileId": data.FileID,
			}, nil); err2 != nil {
				mark(false, true, err2.Error())
				return err2
			}
		}
		ui.ReportUploadProgress(size, size)
		mark(true, false, "")
		drive.ClearUploadSession(sessionKey)
		return nil
	}
	return errors.New("123: 上传会话重试失败")
}

// stopped reports user cancellation (legacy fileui.IsRunning checks).
func stopped(ctx context.Context, ui *model.UploadingUI) bool {
	if ui != nil && ui.IsUploadStopRequested() {
		return true
	}
	return ctx.Err() != nil
}

// markStopped records a user-initiated pause/stop.
func markStopped(ui *model.UploadingUI) {
	if ui == nil {
		return
	}
	ui.Upload.IsDowning = false
	ui.Upload.DownState = "stopped"
}

// ---- expiry extraction (legacy GetExpiresTime) ----

var amzDateRe = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z?$`)

// getExpiresTime extracts an expiry timestamp (ms) from a download url query.
func getExpiresTime(downURL string) int64 {
	rawURL, err := url.QueryUnescape(downURL)
	if err != nil {
		rawURL = downURL
	}
	if rawURL == "" {
		return 0
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	params := map[string]string{}
	for k, vs := range u.Query() {
		lk := strings.ToLower(k)
		if _, ok := params[lk]; !ok && len(vs) > 0 {
			params[lk] = vs[0]
		}
	}
	// AWS 签名直链：X-Amz-Date + X-Amz-Expires
	if amzDate := params["x-amz-date"]; amzDate != "" {
		if amzExpires, err := strconv.ParseFloat(params["x-amz-expires"], 64); err == nil && amzExpires > 0 {
			if m := amzDateRe.FindStringSubmatch(amzDate); len(m) == 7 {
				y, _ := strconv.Atoi(m[1])
				mo, _ := strconv.Atoi(m[2])
				d, _ := strconv.Atoi(m[3])
				h, _ := strconv.Atoi(m[4])
				mi, _ := strconv.Atoi(m[5])
				s, _ := strconv.Atoi(m[6])
				base := time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)
				return base.UnixMilli() + int64(amzExpires*1000)
			}
		}
	}
	for _, key := range []string{"x-oss-expires", "expire", "expires", "expires_at", "exp", "e"} {
		value := params[key]
		if value == "" {
			continue
		}
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			// 纯数字：秒级时间戳必须 >= 2001 年；过小的数字跳过
			if n >= 1_000_000_000 {
				if n < 10_000_000_000 {
					return int64(n * 1000)
				}
				return int64(n)
			}
			continue
		}
		if t := parseLocalTime(value); !t.IsZero() {
			return t.UnixMilli()
		}
	}
	return 0
}
