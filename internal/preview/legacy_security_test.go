package preview

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyCompatibilityUsesOpaqueSessionAndRejectsLegacyRoute(testContext *testing.T) {
	server, err := NewServer(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	defer server.Close()
	local := server.ProxyURL("https://example.com/file?sig=private", map[string]string{"Authorization": "secret"}, "file")
	if local == "" || !strings.Contains(local, "/stream/") || strings.Contains(local, "private") || strings.Contains(local, "secret") || strings.Contains(local, "example.com") {
		testContext.Fatalf("兼容入口仍暴露上游凭据: %s", local)
	}
	request := httptest.NewRequest(http.MethodGet, "/proxy/?t="+server.token+"&u=https://example.com&h=secret", nil)
	response := httptest.NewRecorder()
	server.handleProxy(response, request)
	if response.Code != http.StatusGone {
		testContext.Fatalf("旧路由仍可接收明文凭据: %d", response.Code)
	}
}
