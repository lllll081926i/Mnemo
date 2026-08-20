package preview

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/netx"
)

func TestNewServer(t *testing.T) {
	s, err := NewServer(t.TempDir())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer s.Close()
	if s.Port == 0 {
		t.Fatal("port not assigned")
	}
	if s.token == "" {
		t.Fatal("token not set")
	}
}

func TestValidToken(t *testing.T) {
	s, _ := NewServer()
	defer s.Close()
	r := httptest.NewRequest("GET", "/proxy/?u=http://x.com&t="+s.token, nil)
	if !s.validToken(r) {
		t.Fatal("valid token rejected")
	}
	r2 := httptest.NewRequest("GET", "/proxy/?u=http://x.com&t=wrong", nil)
	if s.validToken(r2) {
		t.Fatal("invalid token accepted")
	}
}

func TestIsSafeProxyURL(t *testing.T) {
	unsafe := []string{
		"http://127.0.0.1/x",
		"http://10.0.0.1/x",
		"http://192.168.1.1/x",
		"http://172.16.0.1/x",
		"http://localhost/x",
		"ftp://example.com/x",
		"http://169.254.1.1/x",
	}
	for _, u := range unsafe {
		if isSafeProxyURL(u) {
			t.Errorf("expected unsafe: %s", u)
		}
	}
	safe := []string{
		"https://example.com/file.mp4",
		"http://123pan.com/api",
		"https://cdn.example.com/path",
	}
	for _, u := range safe {
		if !isSafeProxyURL(u) {
			t.Errorf("expected safe: %s", u)
		}
	}
}

func TestRememberProxyHostOnlyAllowsExplicitLoopback(t *testing.T) {
	s, err := NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.rememberProxyHost("http://192.168.1.20:8080/media")
	if s.isAllowedProxyHost("http://192.168.1.20:8080/media") {
		t.Fatal("private network host must not be auto-allowlisted")
	}
	s.rememberProxyHost("http://127.0.0.1:45678/media")
	if !s.isAllowedProxyHost("http://127.0.0.1:45678/media") {
		t.Fatal("explicit loopback host should remain available for local providers/tests")
	}
}

func TestIsWithinRoots(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewServer(dir)
	defer s.Close()
	if !s.isWithinRoots(dir) {
		t.Error("root itself not within roots")
	}
	if s.isWithinRoots("/etc/passwd") {
		t.Error("/etc/passwd should not be within roots")
	}
}

func TestFilterProxyHeaders(t *testing.T) {
	in := map[string]string{
		"Host":                "x",
		"Connection":          "keep-alive",
		"Authorization":       "Bearer tok",
		"Content-Length":      "100",
		"Proxy-Authorization": "x",
		"Range":               "bytes=0-100",
	}
	out := filterProxyHeaders(in)
	for _, drop := range []string{"host", "connection", "content-length", "proxy-authorization"} {
		if _, ok := out[drop]; ok {
			t.Errorf("header %s should be filtered", drop)
		}
	}
	if out["Authorization"] != "Bearer tok" {
		t.Error("Authorization should be kept")
	}
	if out["Range"] != "bytes=0-100" {
		t.Error("Range should be kept")
	}
}

func TestSanitizeDispositionFilename(t *testing.T) {
	cases := map[string]string{
		"normal.txt":       "normal.txt",
		`has"quote.txt`:    "hasquote.txt",
		`back\slash.txt`:   "backslash.txt",
		"":                 "file",
		"中文文件名.mp4":        "中文文件名.mp4",
		"ctrl\x01char.txt": "ctrlchar.txt",
	}
	for in, want := range cases {
		got := sanitizeDispositionFilename(in)
		if got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProxyRangeAndHead(t *testing.T) {
	data := []byte("0123456789")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "video.mp4", time.Unix(1, 0), bytes.NewReader(data))
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	proxyURL := s.ProxyURL(upstream.URL+"/video.mp4", nil, "演示\"文件.mp4")

	req, err := http.NewRequest(http.MethodGet, proxyURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Fatalf("range response = %d %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "inline") || strings.Contains(got, "\\\"") {
		t.Fatalf("unsafe Content-Disposition = %q", got)
	}

	headReq, err := http.NewRequest(http.MethodHead, proxyURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatal(err)
	}
	headBody, _ := io.ReadAll(headResp.Body)
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK || len(headBody) != 0 {
		t.Fatalf("head response = %d body=%q", headResp.StatusCode, headBody)
	}
}

func TestProxyUsesGlobalNetworkProxy(t *testing.T) {
	previousProxy := netx.GlobalProxy()
	t.Cleanup(func() { netx.SetGlobalProxy(previousProxy) })

	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer target.Close()

	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		if r.URL.String() != target.URL+"/video.mp4" {
			t.Errorf("proxy target = %q, want %q", r.URL.String(), target.URL+"/video.mp4")
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("proxied-video"))
	}))
	defer proxy.Close()
	netx.SetGlobalProxy(proxy.URL)

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	resp, err := http.Get(s.ProxyURL(target.URL+"/video.mp4", nil, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "proxied-video" {
		t.Fatalf("proxy response = %d %q", resp.StatusCode, body)
	}
	if proxyHits != 1 || targetHits != 0 {
		t.Fatalf("proxy hits=%d target hits=%d, want proxy=1 target=0", proxyHits, targetHits)
	}
}

func TestLocalRangeAndHead(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sample.mp4"
	if err := os.WriteFile(path, []byte("abcdefghij"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, s.LocalURL(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.URL.String(), "sample.mp4") || req.URL.Query().Get("p") != "" {
		t.Fatalf("local preview URL exposed the filesystem path: %s", req.URL)
	}
	req.Header.Set("Range", "bytes=3-6")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || string(body) != "defg" {
		t.Fatalf("local range response = %d %q", resp.StatusCode, body)
	}

	headReq, err := http.NewRequest(http.MethodHead, s.LocalURL(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatal(err)
	}
	headBody, _ := io.ReadAll(headResp.Body)
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK || len(headBody) != 0 {
		t.Fatalf("local head response = %d body=%q", headResp.StatusCode, headBody)
	}
}

func TestLocalPreviewRequiresOpaqueGrantAndRootChangesRevokeIt(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	firstPath := firstRoot + "/first.txt"
	secondPath := secondRoot + "/second.txt"
	if err := os.WriteFile(firstPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	firstURL := s.LocalURL(firstPath)
	if firstURL == "" {
		t.Fatal("LocalURL rejected an allowed file")
	}
	direct := s.BaseURL() + "/local/?p=" + url.QueryEscape(firstPath) + "&t=" + url.QueryEscape(s.token)
	resp, err := http.Get(direct)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("path-based local request status = %d, want 404", resp.StatusCode)
	}

	s.SetRoots(secondRoot)
	resp, err = http.Get(firstURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked local grant status = %d, want 404", resp.StatusCode)
	}
	if s.LocalURL(firstPath) != "" {
		t.Fatal("old root remained available after SetRoots")
	}
	if s.LocalURL(secondPath) == "" {
		t.Fatal("new root file was not registered")
	}
}

func TestProxyErrorsAndRedirectProtection(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	badToken := s.BaseURL() + "/proxy/?u=" + "https%3A%2F%2Fexample.com%2Fvideo.mp4&t=wrong"
	resp, err := http.Get(badToken)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token status = %d", resp.StatusCode)
	}

	errorUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream failed", http.StatusServiceUnavailable)
	}))
	defer errorUpstream.Close()
	resp, err = http.Get(s.ProxyURL(errorUpstream.URL, nil, "error.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("upstream error status = %d", resp.StatusCode)
	}

	privateTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("must not be reached"))
	}))
	defer privateTarget.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, nil, privateTarget.URL, http.StatusFound)
	}))
	defer redirector.Close()
	resp, err = http.Get(s.ProxyURL(redirector.URL, nil, "redirect.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("private redirect status = %d", resp.StatusCode)
	}
}

func TestPlaybackSessionKeepsUpstreamDetailsPrivate(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	streamURL, err := s.PlaybackURL(PlaybackSource{
		URL:      upstream.URL + "/video.mp4?signature=secret-value",
		Headers:  map[string]string{"Authorization": "Bearer secret-token"},
		Filename: "video.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(streamURL, upstream.URL) || strings.Contains(streamURL, "secret") {
		t.Fatalf("browser-facing stream URL leaked upstream details: %q", streamURL)
	}
	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "video" {
		t.Fatalf("playback response = %d %q", resp.StatusCode, body)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("upstream Authorization = %q", gotAuth)
	}
}

func TestPlaybackSessionRewritesNestedHLSResources(t *testing.T) {
	var segmentAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nvariant.m3u8\n"))
		case "/variant.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"keys/key.bin?signature=secret\"\n#EXTINF:4,\nsegment.ts?signature=secret\n"))
		case "/segment.ts":
			segmentAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("segment"))
		case "/keys/key.bin":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("key"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	streamURL, err := s.PlaybackURL(PlaybackSource{
		URL:        upstream.URL + "/master.m3u8?signature=top-secret",
		Headers:    map[string]string{"Authorization": "Bearer hls-token"},
		StreamType: "hls",
	})
	if err != nil {
		t.Fatal(err)
	}
	master := mustGetBody(t, streamURL)
	if strings.Contains(master, upstream.URL) || strings.Contains(master, "signature=secret") {
		t.Fatalf("rewritten master playlist leaked upstream URL: %q", master)
	}
	variantURL := firstPlaylistResource(t, master)
	variant := mustGetBody(t, variantURL)
	if strings.Contains(variant, upstream.URL) || strings.Contains(variant, "signature=secret") {
		t.Fatalf("rewritten variant playlist leaked upstream URL: %q", variant)
	}
	if !strings.Contains(variant, "URI=\""+s.BaseURL()+"/stream/") {
		t.Fatalf("HLS key URI was not rewritten: %q", variant)
	}
	segmentURL := firstPlaylistResource(t, variant)
	if got := mustGetBody(t, segmentURL); got != "segment" {
		t.Fatalf("segment body = %q", got)
	}
	if segmentAuth != "Bearer hls-token" {
		t.Fatalf("segment Authorization = %q", segmentAuth)
	}
}

func TestPlaybackSessionRewritesDASHResources(t *testing.T) {
	var initAuth, segmentAuth, initSignature, segmentSignature, segmentTime string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dash/manifest.mpd":
			w.Header().Set("Content-Type", "application/dash+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MPD><Period><AdaptationSet mimeType="video/mp4"><Representation id="video"><SegmentTemplate initialization="init.mp4?signature=init-secret" media="segments/chunk-$Number$.m4s?signature=segment-secret&amp;time=$Time$" startNumber="1" /></Representation></AdaptationSet></Period></MPD>`))
		case "/dash/init.mp4":
			initAuth = r.Header.Get("Authorization")
			initSignature = r.URL.Query().Get("signature")
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("init"))
		case "/dash/segments/chunk-1.m4s":
			segmentAuth = r.Header.Get("Authorization")
			segmentSignature = r.URL.Query().Get("signature")
			segmentTime = r.URL.Query().Get("time")
			w.Header().Set("Content-Type", "video/iso.segment")
			_, _ = w.Write([]byte("segment"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	streamURL, err := s.PlaybackURL(PlaybackSource{
		URL:        upstream.URL + "/dash/manifest.mpd?signature=manifest-secret",
		Headers:    map[string]string{"Authorization": "Bearer dash-token"},
		StreamType: "dash",
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := mustGetBody(t, streamURL)
	if strings.Contains(manifest, upstream.URL) || strings.Contains(manifest, "signature=") {
		t.Fatalf("rewritten DASH manifest leaked upstream details: %q", manifest)
	}
	var mpd struct {
		SegmentTemplate struct {
			Initialization string `xml:"initialization,attr"`
			Media          string `xml:"media,attr"`
		} `xml:"Period>AdaptationSet>Representation>SegmentTemplate"`
	}
	if err := xml.Unmarshal([]byte(manifest), &mpd); err != nil {
		t.Fatalf("parse rewritten DASH manifest: %v", err)
	}
	if !strings.Contains(mpd.SegmentTemplate.Initialization, "/stream/") || !strings.Contains(mpd.SegmentTemplate.Media, "/stream/") {
		t.Fatalf("DASH resources were not rewritten: %+v", mpd.SegmentTemplate)
	}
	if got := mustGetBody(t, mpd.SegmentTemplate.Initialization); got != "init" {
		t.Fatalf("init body = %q", got)
	}
	if !strings.Contains(mpd.SegmentTemplate.Media, "$Time$") {
		t.Fatalf("DASH query template was not preserved locally: %q", mpd.SegmentTemplate.Media)
	}
	segmentURL := strings.Replace(mpd.SegmentTemplate.Media, "$Number$", "1", 1)
	segmentURL = strings.Replace(segmentURL, "$Time$", "5000", 1)
	if got := mustGetBody(t, segmentURL); got != "segment" {
		t.Fatalf("segment body = %q", got)
	}
	if initAuth != "Bearer dash-token" || segmentAuth != "Bearer dash-token" {
		t.Fatalf("DASH Authorization headers = init:%q segment:%q", initAuth, segmentAuth)
	}
	if initSignature != "init-secret" || segmentSignature != "segment-secret" || segmentTime != "5000" {
		t.Fatalf("DASH signed queries = init:%q segment:%q time:%q", initSignature, segmentSignature, segmentTime)
	}
}

func TestPlaybackSessionConvertsSRTSubtitle(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/x-subrip")
		_, _ = w.Write([]byte("1\r\n00:00:01,000 --> 00:00:02,500\r\n字幕内容\r\n"))
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	streamURL, err := s.PlaybackURL(PlaybackSource{
		URL:        upstream.URL + "/captions.srt?signature=subtitle-secret",
		Headers:    map[string]string{"Authorization": "Bearer subtitle-token"},
		StreamType: "subtitle-srt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(streamURL, "subtitle-secret") {
		t.Fatalf("subtitle stream URL leaked upstream details: %q", streamURL)
	}
	resp, err := http.Get(streamURL)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/vtt") {
		t.Fatalf("subtitle response = %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if got := string(body); !strings.HasPrefix(got, "WEBVTT\n\n") || strings.Contains(got, "00:00:01,000") || !strings.Contains(got, "00:00:01.000 --> 00:00:02.500") {
		t.Fatalf("converted subtitle = %q", got)
	}
	if gotAuth != "Bearer subtitle-token" {
		t.Fatalf("subtitle Authorization = %q", gotAuth)
	}
}

func TestPlaybackSessionRefreshesAfterAdaptiveResourceForbidden(t *testing.T) {
	refreshes := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:4,\nold.ts\n"))
		case "/old.ts":
			http.Error(w, "expired", http.StatusForbidden)
		case "/fresh.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:4,\nfresh.ts\n"))
		case "/fresh.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write([]byte("fresh"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	streamURL, err := s.PlaybackURL(PlaybackSource{
		URL:        upstream.URL + "/old.m3u8",
		StreamType: "hls",
		Refresh: func(context.Context) (PlaybackSource, error) {
			refreshes++
			return PlaybackSource{URL: upstream.URL + "/fresh.m3u8", StreamType: "hls"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldManifest := mustGetBody(t, streamURL)
	oldSegment := firstPlaylistResource(t, oldManifest)
	resp, err := http.Get(oldSegment)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("old segment status = %d", resp.StatusCode)
	}
	if refreshes != 1 {
		t.Fatalf("refreshes after forbidden segment = %d", refreshes)
	}
	freshManifest := mustGetBody(t, streamURL)
	if strings.Contains(freshManifest, "old.ts") {
		t.Fatalf("root manifest was not refreshed: %q", freshManifest)
	}
	freshSegment := firstPlaylistResource(t, freshManifest)
	if got := mustGetBody(t, freshSegment); got != "fresh" {
		t.Fatalf("fresh segment body = %q", got)
	}
}

func TestPlaybackSessionRefreshesExpiredURLAndKeepsRange(t *testing.T) {
	initialCalls := 0
	freshCalls := 0
	var refreshedRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/expired.mp4":
			initialCalls++
			http.Error(w, "expired", http.StatusForbidden)
		case "/fresh.mp4":
			freshCalls++
			refreshedRange = r.Header.Get("Range")
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("fresh-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	streamURL, err := s.PlaybackURL(PlaybackSource{
		URL: upstream.URL + "/expired.mp4",
		Refresh: func(context.Context) (PlaybackSource, error) {
			return PlaybackSource{URL: upstream.URL + "/fresh.mp4"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=8-")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || string(body) != "fresh-bytes" {
		t.Fatalf("refreshed response = %d %q", resp.StatusCode, body)
	}
	if initialCalls != 1 || freshCalls != 1 {
		t.Fatalf("upstream calls expired=%d fresh=%d", initialCalls, freshCalls)
	}
	if refreshedRange != "bytes=8-" {
		t.Fatalf("refreshed Range = %q", refreshedRange)
	}
}

func mustGetBody(t *testing.T, rawURL string) string {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%q", rawURL, resp.StatusCode, body)
	}
	return string(body)
}

func firstPlaylistResource(t *testing.T, playlist string) string {
	t.Helper()
	for _, line := range strings.Split(playlist, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http://127.0.0.1:") {
			return line
		}
	}
	t.Fatalf("playlist does not contain a local resource URL: %q", playlist)
	return ""
}

var _ = http.MethodGet
