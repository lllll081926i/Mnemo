package dlengine

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

const probeModified = "Mon, 05 Oct 2026 02:37:34 GMT"
const rangeModified = "Sun, 04 Oct 2026 01:02:03 GMT"

func TestResourceValidatorPrefersMatchingStrongETag(t *testing.T) {
	for _, sample := range []struct {
		name     string
		expected resourceValidator
		etag     string
		modified string
		wantErr  bool
	}{
		{name: "强 ETag 相同，时间不同", expected: resourceValidator{ETag: `"stable"`, LastModified: probeModified}, etag: `"stable"`, modified: rangeModified},
		{name: "强 ETag 改变", expected: resourceValidator{ETag: `"stable"`, LastModified: probeModified}, etag: `"changed"`, modified: probeModified, wantErr: true},
		{name: "弱 ETag 不覆盖日期", expected: resourceValidator{ETag: `W/"stable"`, LastModified: probeModified}, etag: `W/"stable"`, modified: rangeModified, wantErr: true},
		{name: "ETag 缺失仍检查日期", expected: resourceValidator{ETag: `"stable"`, LastModified: probeModified}, modified: rangeModified, wantErr: true},
		{name: "相同日期不同格式", expected: resourceValidator{LastModified: probeModified}, modified: "Monday, 05-Oct-26 02:37:34 GMT"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			header := http.Header{}
			header.Set("ETag", sample.etag)
			header.Set("Last-Modified", sample.modified)
			if err := sample.expected.verifyResponse(header); (err != nil) != sample.wantErr {
				t.Fatalf("校验结果 = %v，期望错误 = %v", err, sample.wantErr)
			}
		})
	}
	if !resumeIdentityMatches(state{LastModified: probeModified}, resourceValidator{LastModified: "Monday, 05-Oct-26 02:37:34 GMT"}) {
		t.Fatal("相同 HTTP 日期的不同格式不应废弃断点")
	}
}

func TestLastModifiedMismatchRestartsSafely(t *testing.T) {
	for _, sample := range []struct {
		name            string
		allow           bool
		etag            string
		rangeETag       string
		fullETag        string
		fullSizeOffset  int
		rangeSizeOffset int
		minSize         int64
		wantFull        int32
		wantErr         bool
	}{
		{name: "时间漂移整流重下", allow: true, wantFull: 1},
		{name: "相同强 ETag 保留分段", allow: true, etag: `"stable"`, rangeETag: `"stable"`},
		{name: "未开启策略仍拒绝", wantErr: true},
		{name: "ETag 改变不得恢复", allow: true, etag: `"stable"`, rangeETag: `"changed"`, wantErr: true},
		{name: "整流仍校验 ETag", allow: true, etag: `"stable"`, fullETag: `"changed"`, wantFull: 1, wantErr: true},
		{name: "整流仍校验长度", allow: true, fullSizeOffset: -1, wantFull: 1, wantErr: true},
		{name: "分段长度改变不得恢复", allow: true, rangeSizeOffset: 1, wantErr: true},
		{name: "小文件同样恢复", allow: true, minSize: 1 << 20, wantFull: 2},
	} {
		t.Run(sample.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("guangya"), 4096)
			var fullRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("User-Agent") != "guangya-download" || request.Header.Get("Referer") != "https://www.guangyapan.com/" {
					t.Error("恢复下载丢失网盘请求头")
				}
				if request.Header.Get("Range") == "" {
					attempt := fullRequests.Add(1)
					writer.Header().Set("ETag", sample.fullETag)
					writer.Header().Set("Last-Modified", rangeModified)
					if sample.etag != "" && request.Header.Get("If-Match") != sample.etag {
						t.Error("整流下载丢失 ETag 条件")
					}
					if sample.minSize == 0 || attempt > 1 {
						if request.Header.Get("If-Unmodified-Since") != "" {
							t.Error("整流恢复仍使用不稳定的修改时间")
						}
					}
					body := payload[:len(payload)+sample.fullSizeOffset]
					writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
					_, _ = writer.Write(body)
					return
				}
				start, end, ok := parseTestRange(request.Header.Get("Range"))
				if !ok {
					t.Error("无效 Range 请求")
					writer.WriteHeader(400)
					return
				}
				if start == 0 && end == 0 {
					writer.Header().Set("ETag", sample.etag)
					writer.Header().Set("Last-Modified", probeModified)
				} else {
					writer.Header().Set("ETag", sample.rangeETag)
					writer.Header().Set("Last-Modified", rangeModified)
				}
				total := int64(len(payload))
				if end != 0 {
					total += int64(sample.rangeSizeOffset)
				}
				writeTestRange(writer, start, end, total, payload[start:end+1])
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "image.esd")
			var progress Progress
			err := Download(t.Context(), Options{Concurrency: 8, ChunkSize: 4096, MinSize: max(1, sample.minSize), AllowLastModifiedFallback: sample.allow, Headers: map[string]string{"User-Agent": "guangya-download", "Referer": "https://www.guangyapan.com/"}}, server.URL, path, func(value Progress) { progress = value })
			if (err != nil) != sample.wantErr || fullRequests.Load() != sample.wantFull {
				t.Fatalf("下载结果 = %v，整流次数 = %d，期望错误 = %v，次数 = %d", err, fullRequests.Load(), sample.wantErr, sample.wantFull)
			}
			if sample.wantErr {
				if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatal("校验失败却提交了最终文件")
				}
				return
			}
			actual, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(actual, payload) || progress.Percent != 100 || progress.Downloaded != int64(len(payload)) {
				t.Fatalf("恢复后的内容或进度不正确: %v, %+v", readErr, progress)
			}
		})
	}
}

func TestInterruptedLastModifiedFallbackInvalidatesCheckpoint(t *testing.T) {
	payload := bytes.Repeat([]byte("guangya"), 2048)
	var interrupted atomic.Bool
	interrupted.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Range") == "" {
			writer.Header().Set("Last-Modified", rangeModified)
			writer.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			if interrupted.Load() {
				_, _ = writer.Write(payload[:100])
				return
			}
			_, _ = writer.Write(payload)
			return
		}
		start, end, _ := parseTestRange(request.Header.Get("Range"))
		writer.Header().Set("Last-Modified", rangeModified)
		if end == 0 {
			writer.Header().Set("Last-Modified", probeModified)
		}
		writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path+".part", bytes.Repeat([]byte{'!'}, len(payload)), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := &state{URLHash: urlFingerprint(server.URL), Total: int64(len(payload)), Chunk: 4096, Done: []bool{true, false, false, false}, LastModified: probeModified}
	if err := persistState(path+".state.json", checkpoint); err != nil {
		t.Fatal(err)
	}
	opts := Options{Concurrency: 8, ChunkSize: 4096, MinSize: 1, AllowLastModifiedFallback: true}
	if err := Download(t.Context(), opts, server.URL, path, nil); err == nil {
		t.Fatal("整流中断未报错")
	}
	if _, err := os.Stat(path + ".state.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("整流中断后仍保留旧分片断点")
	}
	interrupted.Store(false)
	if err := Download(t.Context(), opts, server.URL, path, nil); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("后续下载混入了旧分片内容")
	}
}
