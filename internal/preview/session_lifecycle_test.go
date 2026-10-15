package preview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func deletePreview(testContext *testing.T, target string, expected int) {
	testContext.Helper()
	request, err := http.NewRequestWithContext(testContext.Context(), http.MethodDelete, target, nil)
	if err != nil {
		testContext.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		testContext.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != expected {
		testContext.Fatalf("会话释放状态 = %d，期望 %d", response.StatusCode, expected)
	}
}

func TestPlaybackReleaseIsAuthenticatedScopedAndIdempotent(testContext *testing.T) {
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	target, err := server.PlaybackURL(PlaybackSource{URL: "https://example.com/video.mp4?signature=secret", Headers: map[string]string{"Authorization": "private"}})
	if err != nil {
		testContext.Fatal(err)
	}
	parsed, _ := url.Parse(target)
	id := strings.TrimPrefix(parsed.Path, "/stream/")
	session := server.getPlaybackSession(id)
	resourceURL := server.playbackResourceURL(id, session, "https://example.com/segment.ts?signature=secret")
	deletePreview(testContext, strings.ReplaceAll(target, "t="+server.token, "t=wrong"), http.StatusForbidden)
	deletePreview(testContext, resourceURL, http.StatusBadRequest)
	if server.getPlaybackSession(id) == nil {
		testContext.Fatal("无效释放请求撤销了会话")
	}
	deletePreview(testContext, target, http.StatusNoContent)
	deletePreview(testContext, target, http.StatusNoContent)
	if server.getPlaybackSession(id) != nil {
		testContext.Fatal("释放后会话仍可查找")
	}
	response, err := http.Get(resourceURL)
	if err != nil {
		testContext.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusGone {
		testContext.Fatalf("旧资源仍可访问: %d", response.StatusCode)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.closed || session.source.URL != "" || len(session.source.Headers) != 0 || len(session.resources) != 0 {
		testContext.Fatal("撤销后未释放源凭据或资源索引")
	}
}

func TestPlaybackReleaseCancelsActiveRefresh(testContext *testing.T) {
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	started := make(chan struct{})
	target, err := server.PlaybackURL(PlaybackSource{
		URL: "https://example.com/expired.mp4", ExpiresAt: time.Now().Add(-time.Hour),
		Refresh: func(ctx context.Context) (PlaybackSource, error) {
			close(started)
			<-ctx.Done()
			return PlaybackSource{URL: "https://example.com/late.mp4"}, nil
		},
	})
	if err != nil {
		testContext.Fatal(err)
	}
	parsed, _ := url.Parse(target)
	session := server.getPlaybackSession(strings.TrimPrefix(parsed.Path, "/stream/"))
	finished := make(chan error, 1)
	go func() { _, err := session.resolve(testContext.Context(), false); finished <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		testContext.Fatal("续签未开始")
	}
	deletePreview(testContext, target, http.StatusNoContent)
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			testContext.Fatalf("释放后续签仍被接受: %v", err)
		}
	case <-time.After(3 * time.Second):
		testContext.Fatal("释放没有取消续签")
	}
	if _, err := session.resolve(testContext.Context(), false); !errors.Is(err, context.Canceled) {
		testContext.Fatalf("已释放会话被复用: %v", err)
	}
}

func TestPlaybackReleaseCancelsActiveUpstream(testContext *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "video/mp4")
		_, _ = writer.Write([]byte("video"))
		writer.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
		close(finished)
	}))
	defer upstream.Close()
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	target, err := server.PlaybackURL(PlaybackSource{URL: upstream.URL + "/video.mp4"})
	if err != nil {
		testContext.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		response, err := client.Get(target)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		server.Close()
		testContext.Fatal("上游读取未开始")
	}
	deletePreview(testContext, target, http.StatusNoContent)
	for _, done := range []<-chan struct{}{finished, readerDone} {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			server.Close()
			testContext.Fatal("释放后网络读取仍未退出")
		}
	}
}

func TestPlaybackRepeatedReleaseAndServerCloseClearSessions(testContext *testing.T) {
	server, err := NewServer()
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	for index := range 1024 {
		target, err := server.PlaybackURL(PlaybackSource{URL: fmt.Sprintf("https://example.com/%d.mp4", index)})
		if err != nil {
			testContext.Fatal(err)
		}
		parsed, _ := url.Parse(target)
		server.releasePlaybackSession(strings.TrimPrefix(parsed.Path, "/stream/"))
	}
	if len(server.sessions) != 0 {
		testContext.Fatalf("反复打开关闭后会话仍在增长: %d", len(server.sessions))
	}
	if _, err := server.PlaybackURL(PlaybackSource{URL: "https://example.com/final.mp4"}); err != nil {
		testContext.Fatal(err)
	}
	if err := server.Close(); err != nil {
		testContext.Fatal(err)
	}
	if len(server.sessions) != 0 {
		testContext.Fatal("服务器退出没有清空会话")
	}
	if _, err := server.PlaybackURL(PlaybackSource{URL: "https://example.com/closed.mp4"}); err == nil {
		testContext.Fatal("服务器关闭后仍注册会话")
	}
}
