package dlengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadPreservesProviderHeaders(t *testing.T) {
	for _, explicit := range []string{"", "explicit-UA"} {
		t.Run(explicit, func(t *testing.T) {
			var calls atomic.Int32
			want := "Dalvik/provider-UA"
			if explicit != "" {
				want = explicit
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Header.Get("User-Agent") != want || request.Header.Get("Referer") != "https://provider.example/" || request.Header.Get("Accept-Encoding") != "identity" {
					t.Errorf("下载头不正确: %+v", request.Header)
				}
				http.ServeContent(writer, request, "file", time.Time{}, strings.NewReader("payload"))
			}))
			defer server.Close()
			opts := Options{Concurrency: 2, ChunkSize: 2, MinSize: 1, UserAgent: explicit, Headers: map[string]string{"user-agent": "Dalvik/provider-UA", "Referer": "https://provider.example/"}}
			if err := Download(t.Context(), opts, server.URL, filepath.Join(t.TempDir(), "file"), nil); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 5 {
				t.Fatalf("请求数 %d，期望 5", calls.Load())
			}
		})
	}
}

func TestHTTPReadIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "100")
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	client := NewHTTPClient(Options{ReadIdleTimeout: 40 * time.Millisecond})
	defer client.CloseIdleConnections()
	if client.Timeout != 0 {
		t.Fatal("大文件下载不应设置总超时")
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, ErrReadIdleTimeout) {
		t.Fatalf("空闲读取未超时: %v", err)
	}
}

func TestIdleTimeoutDoesNotIncludeConsumerDelay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "payload")
	}))
	defer server.Close()
	client := NewHTTPClient(Options{ReadIdleTimeout: 50 * time.Millisecond})
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	first := make([]byte, 1)
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	rest, err := io.ReadAll(response.Body)
	if err != nil || string(first)+string(rest) != "payload" {
		t.Fatalf("消费方等待被误判为空闲: %q %v", rest, err)
	}
}

func TestSharedLimiterWakesBlockedReadsWhenUncapped(t *testing.T) {
	limiter := NewSharedLimiter(1)
	done := make(chan error, 1)
	go func() { done <- limiter.Wait(t.Context(), 100) }()
	time.Sleep(25 * time.Millisecond)
	limiter.SetRate(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("取消限速后已有等待仍未释放")
	}
}

func TestSharedLimiterKeepsAggregateCapAcrossWindowReset(t *testing.T) {
	limiter := NewSharedLimiter(200)
	done := make(chan error, 4)
	started := time.Now()
	for worker := 0; worker < 3; worker++ {
		go func() { done <- limiter.Wait(t.Context(), 100) }()
	}
	go func() {
		time.Sleep(1100 * time.Millisecond)
		done <- limiter.Wait(t.Context(), 100)
	}()
	for worker := 0; worker < 4; worker++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(started) < 1900*time.Millisecond {
		t.Fatal("多个连接重复领取了同一时间窗口的额度")
	}
}

func TestPermanentRangeErrorsAreNotRetried(t *testing.T) {
	for _, status := range []int{401, 403, 404, 416} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var ranges atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Range") == "bytes=0-0" {
					writer.Header().Set("Content-Range", "bytes 0-0/16")
					writer.Header().Set("Content-Length", "1")
					writer.WriteHeader(http.StatusPartialContent)
					_, _ = io.WriteString(writer, "x")
					return
				}
				ranges.Add(1)
				writer.WriteHeader(status)
			}))
			defer server.Close()
			err := Download(t.Context(), Options{Concurrency: 1, MinSize: 1, ChunkSize: 16}, server.URL, filepath.Join(t.TempDir(), "file"), nil)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("http %d", status)) || ranges.Load() != 1 {
				t.Fatalf("永久错误重复请求: 请求=%d，错误=%v", ranges.Load(), err)
			}
		})
	}
}

func TestHTTPRetryAfterAndCancel(t *testing.T) {
	for _, status := range []int{429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if calls.Add(1) == 1 {
					writer.Header().Set("Retry-After", "1")
					writer.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(writer, "file")
			}))
			defer server.Close()
			client := NewHTTPClient(Options{})
			defer client.CloseIdleConnections()
			started := time.Now()
			response, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if calls.Load() != 2 || time.Since(started) < time.Second {
				t.Fatal("没有遵守 Retry-After")
			}
			calls.Store(0)
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			_, err = client.Do(request)
			if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
				t.Fatalf("重试等待不能取消: %v，请求 %d", err, calls.Load())
			}
		})
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	for header, want := range map[string]time.Duration{
		"2": 2 * time.Second,
		now.Add(3 * time.Second).Format(http.TimeFormat): 3 * time.Second,
		"invalid":    0,
		"-1":         0,
		"9999999999": 121 * time.Second,
	} {
		if got := retryAfter(header, now); got != want {
			t.Fatalf("Retry-After %q: %v，期望 %v", header, got, want)
		}
	}
}

func TestRetryRefreshesRequestBoundAuth(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := calls.Add(1)
		if request.Header.Get("X-Nonce-Count") != fmt.Sprint(count) {
			t.Error("重试复用了旧的请求鉴权")
		}
		if count == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(writer, "file")
	}))
	defer server.Close()
	var signatures atomic.Int32
	opts := Options{RequestAuth: func(request *http.Request) error {
		request.Header.Set("X-Nonce-Count", fmt.Sprint(signatures.Add(1)))
		return nil
	}}
	request, err := NewRequest(t.Context(), http.MethodGet, server.URL, opts)
	if err != nil {
		t.Fatal(err)
	}
	client := NewHTTPClient(opts)
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if calls.Load() != 2 || signatures.Load() != 2 {
		t.Fatal("没有为每次请求重新签名")
	}
}

func TestRetryWaitIsSharedAcrossConnections(t *testing.T) {
	limited := make(chan struct{})
	served := make(chan time.Time, 1)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/limited" && calls.Add(1) == 1 {
			writer.Header().Set("Retry-After", "1")
			writer.WriteHeader(http.StatusTooManyRequests)
			close(limited)
			return
		}
		if request.URL.Path == "/other" {
			served <- time.Now()
		}
		_, _ = io.WriteString(writer, "file")
	}))
	defer server.Close()
	client := NewHTTPClient(Options{})
	defer client.CloseIdleConnections()
	finished := make(chan error, 1)
	started := time.Now()
	go func() {
		response, err := client.Get(server.URL + "/limited")
		if err == nil {
			response.Body.Close()
		}
		finished <- err
	}()
	<-limited
	transport := client.Transport.(*downloadTransport)
	deadline := time.Now().Add(time.Second)
	for {
		transport.mu.Lock()
		waiting := transport.notBefore.After(started)
		transport.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("没有建立共享退避")
		}
		time.Sleep(time.Millisecond)
	}
	response, err := client.Get(server.URL + "/other")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if at := <-served; at.Sub(started) < time.Second {
		t.Fatal("其他连接绕过了 Retry-After")
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestLongRetryAfterDoesNotRetryEarly(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Retry-After", "3600")
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := NewHTTPClient(Options{})
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if calls.Load() != 1 || response.StatusCode != http.StatusTooManyRequests {
		t.Fatal("长时间冷却应返回错误而不是提前重试")
	}
}

func TestCheckpointFailureIsReported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.ServeContent(writer, request, "file", time.Time{}, strings.NewReader("payload"))
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "file")
	if err := os.Mkdir(destination+".state.json", 0700); err != nil {
		t.Fatal(err)
	}
	err := Download(t.Context(), Options{MinSize: 1, ChunkSize: 2}, server.URL, destination, nil)
	if err == nil {
		t.Fatal("检查点写入失败被隐藏")
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("失败下载不能提交为成品")
	}
}

func TestAutoChunkResumeKeepsExistingLayout(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), (2<<20)/16)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Range") != "bytes=0-0" {
			requests.Add(1)
			var start, end int64
			_, _ = fmt.Sscanf(request.Header.Get("Range"), "bytes=%d-%d", &start, &end)
			if start < 1<<20 {
				t.Error("已完成的分片被重复下载")
			}
		}
		writer.Header().Set("ETag", `"stable"`)
		http.ServeContent(writer, request, "file", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "file")
	partial := make([]byte, len(payload))
	copy(partial, payload[:1<<20])
	if err := os.WriteFile(destination+".part", partial, 0600); err != nil {
		t.Fatal(err)
	}
	previous := &state{URLHash: urlFingerprint(server.URL), Total: int64(len(payload)), Chunk: 256 << 10, Done: []bool{true, true, true, true, false, false, false, false}, ETag: `"stable"`}
	if err := persistState(destination+".state.json", previous); err != nil {
		t.Fatal(err)
	}
	if err := Download(t.Context(), Options{Concurrency: 8, MinSize: 1}, server.URL, destination, nil); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(actual, payload) || requests.Load() != 4 {
		t.Fatalf("恢复分片布局失败: 请求 %d，错误 %v", requests.Load(), err)
	}
}

func TestDownloadParallelPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过受控性能测试")
	}
	payload := bytes.Repeat([]byte("0123456789abcdef"), 2<<20)
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var start, end int64
		if _, err := fmt.Sscanf(request.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(writer, "Range required", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		writer.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		writer.WriteHeader(http.StatusPartialContent)
		if end == 0 {
			_, _ = writer.Write(payload[:1])
			return
		}
		connections := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); connections > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, connections) {
				break
			}
		}
		for offset := start; offset <= end; offset += 256 << 10 {
			select {
			case <-request.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
			next := min(offset+(256<<10), end+1)
			if _, err := writer.Write(payload[offset:next]); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	var durations [2]time.Duration
	for index, connections := range []int{1, 8} {
		peak.Store(0)
		destination := filepath.Join(t.TempDir(), "file")
		started := time.Now()
		if err := Download(t.Context(), Options{Concurrency: connections}, server.URL, destination, nil); err != nil {
			t.Fatal(err)
		}
		durations[index] = time.Since(started)
		actual, err := os.ReadFile(destination)
		if err != nil || !bytes.Equal(actual, payload) {
			t.Fatal("并行下载内容不一致")
		}
		if peak.Load() != int32(connections) {
			t.Fatalf("有效并发 %d，期望 %d", peak.Load(), connections)
		}
		t.Logf("32 MiB，%d 连接：%s，%.2f MiB/s", connections, durations[index], 32/durations[index].Seconds())
	}
	t.Logf("受控单连接限速场景提速 %.2f 倍；不代表网盘公网速度", float64(durations[0])/float64(durations[1]))
}
