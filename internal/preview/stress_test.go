package preview

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlaybackConcurrentRefreshAndResourceChurn(testContext *testing.T) {
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	var refreshes atomic.Int32
	streamURL, err := server.PlaybackURL(PlaybackSource{
		URL:       "https://example.com/旧视频.mp4?signature=secret",
		ExpiresAt: time.Now().Add(-time.Hour),
		Refresh: func(context.Context) (PlaybackSource, error) {
			refreshes.Add(1)
			return PlaybackSource{URL: "https://example.com/fresh.mp4", ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	})
	if err != nil {
		testContext.Fatal(err)
	}
	parsed, _ := url.Parse(streamURL)
	sessionID := strings.TrimPrefix(parsed.Path, "/stream/")
	session := server.getPlaybackSession(sessionID)
	var workers sync.WaitGroup
	for worker := range 32 {
		workers.Go(func() {
			for index := range 128 {
				source, err := session.resolve(testContext.Context(), false)
				if err != nil || source.URL != "https://example.com/fresh.mp4" {
					testContext.Errorf("并发续签失败: %v %v", source, err)
					return
				}
				if index%2 == 0 {
					server.playbackResourceURL(sessionID, session, fmt.Sprintf("https://example.com/%d/%d.ts?key=secret", worker, index))
				} else {
					session.dashResourceID("https://example.com", fmt.Sprintf("track=%d&segment=%d", worker, index))
				}
			}
		})
	}
	workers.Wait()
	if refreshes.Load() != 1 {
		testContext.Fatalf("并发续签重复请求: %d", refreshes.Load())
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.resourceOrder) != maxPlaybackResources || len(session.resources)+len(session.dashResources) != maxPlaybackResources || len(session.resourceIDs) != len(session.resources) || len(session.dashResourceIDs) != len(session.dashResources) {
		testContext.Fatalf("资源索引未保持有界一致: order=%d hls=%d dash=%d", len(session.resourceOrder), len(session.resources), len(session.dashResources))
	}
	for target, id := range session.resourceIDs {
		if session.resources[id] != target {
			testContext.Fatal("HLS 正反索引不一致")
		}
	}
	for key, id := range session.dashResourceIDs {
		resource := session.dashResources[id]
		if key != resource.BaseURL+"\x00"+resource.RawQuery {
			testContext.Fatal("DASH 正反索引不一致")
		}
	}
}

func TestPlaybackSlowRefreshDoesNotBlockRegistrationOrCancellation(testContext *testing.T) {
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(testContext.Context())
	defer cancel()
	streamURL, err := server.PlaybackURL(PlaybackSource{
		URL: "https://example.com/expired.mp4", ExpiresAt: time.Now().Add(-time.Hour),
		Refresh: func(ctx context.Context) (PlaybackSource, error) {
			close(started)
			<-ctx.Done()
			return PlaybackSource{}, ctx.Err()
		},
	})
	if err != nil {
		testContext.Fatal(err)
	}
	parsed, _ := url.Parse(streamURL)
	session := server.getPlaybackSession(strings.TrimPrefix(parsed.Path, "/stream/"))
	finished := make(chan error, 1)
	go func() { _, err := session.resolve(ctx, false); finished <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		testContext.Fatal("续签未开始")
	}
	registered := make(chan error, 1)
	go func() {
		_, err := server.PlaybackURL(PlaybackSource{URL: "https://example.com/other.mp4"})
		registered <- err
	}()
	select {
	case err := <-registered:
		if err != nil {
			testContext.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		testContext.Fatal("慢续签阻塞其他预览注册")
	}
	waitCtx, stopWaiting := context.WithCancel(testContext.Context())
	waiter := make(chan error, 1)
	go func() { _, err := session.resolve(waitCtx, false); waiter <- err }()
	stopWaiting()
	select {
	case err := <-waiter:
		if !errors.Is(err, context.Canceled) {
			testContext.Fatalf("等待续签未保留取消: %v", err)
		}
	case <-time.After(3 * time.Second):
		testContext.Fatal("续签等待不能取消")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			testContext.Fatalf("活动续签未取消: %v", err)
		}
	case <-time.After(3 * time.Second):
		testContext.Fatal("活动续签没有结束")
	}
}

func TestPlaybackRejectedRevisionRefreshesOnlyOnce(testContext *testing.T) {
	var calls atomic.Int32
	session := &playbackSession{source: PlaybackSource{
		URL: "https://example.com/video.mp4", Headers: map[string]string{"Authorization": "old"},
		Refresh: func(context.Context) (PlaybackSource, error) {
			calls.Add(1)
			return PlaybackSource{URL: "https://example.com/video.mp4", Headers: map[string]string{"Authorization": "fresh"}}, nil
		},
	}}
	rejected, err := session.resolve(testContext.Context(), false)
	if err != nil {
		testContext.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 64 {
		workers.Go(func() {
			fresh, err := session.refreshRejected(testContext.Context(), rejected)
			if err != nil || fresh.Headers["Authorization"] != "fresh" {
				testContext.Errorf("拒绝回执续签失败: %v %v", fresh, err)
			}
		})
	}
	workers.Wait()
	if calls.Load() != 1 {
		testContext.Fatalf("同一旧版本重复续签: %d", calls.Load())
	}
}

func TestPlaybackConcurrentDisconnectReleasesUpstream(testContext *testing.T) {
	const requestCount = 16
	started := make(chan struct{}, requestCount)
	finished := make(chan struct{}, requestCount)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "video/mp4")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write([]byte("video"))
		writer.(http.Flusher).Flush()
		started <- struct{}{}
		<-request.Context().Done()
		finished <- struct{}{}
	}))
	defer upstream.Close()
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	streamURL, err := server.PlaybackURL(PlaybackSource{URL: upstream.URL + "/视频.mp4"})
	if err != nil {
		testContext.Fatal(err)
	}
	ctx, cancel := context.WithCancel(testContext.Context())
	defer cancel()
	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	var workers sync.WaitGroup
	for range requestCount {
		workers.Go(func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
			if err != nil {
				testContext.Error(err)
				return
			}
			request.Header.Set("Range", "bytes=0-")
			response, err := client.Do(request)
			if err == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
			}
		})
	}
	for range requestCount {
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			cancel()
			server.Close()
			testContext.Fatal("并发流未全部开始")
		}
	}
	cancel()
	workers.Wait()
	for range requestCount {
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			server.Close()
			testContext.Fatal("关闭预览后上游连接仍在读取")
		}
	}
}

func TestPlaylistLimitRejectsTruncationAndCompressedExpansion(testContext *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, size := range []int{8 << 20, (8 << 20) + 1} {
			testContext.Run(fmt.Sprintf("gzip=%v/size=%d", compressed, size), func(testContext *testing.T) {
				body := bytes.Repeat([]byte("a"), size)
				header := http.Header{}
				if compressed {
					var buffer bytes.Buffer
					writer := gzip.NewWriter(&buffer)
					_, _ = writer.Write(body)
					if err := writer.Close(); err != nil {
						testContext.Fatal(err)
					}
					body = buffer.Bytes()
					header.Set("Content-Encoding", "gzip")
				}
				response := &http.Response{Body: io.NopCloser(bytes.NewReader(body)), Header: header}
				result, err := readPlaylistBody(response)
				if size > 8<<20 {
					if err == nil || result != nil {
						testContext.Fatal("超限正文被截断后作为成功返回")
					}
				} else if err != nil || len(result) != size {
					testContext.Fatalf("边界内正文读取失败: %d %v", len(result), err)
				}
			})
		}
	}
}

func BenchmarkPlaybackResourceChurn(benchmark *testing.B) {
	server := &Server{token: "benchmark-token"}
	session := &playbackSession{
		resources: map[string]string{}, resourceIDs: map[string]string{},
		dashResources: map[string]dashPlaybackResource{}, dashResourceIDs: map[string]string{},
	}
	benchmark.ReportAllocs()
	for index := range benchmark.N {
		server.playbackResourceURL("session", session, fmt.Sprintf("https://example.com/%d.ts?signature=secret", index))
	}
	benchmark.StopTimer()
	if len(session.resources) > maxPlaybackResources || len(session.resourceOrder) > maxPlaybackResources {
		benchmark.Fatal("资源索引超过上限")
	}
}
