package pikpak

import (
	"net/http"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestSignedDownloadDoesNotForwardAccountToken(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = pikpakRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer account-secret" {
			t.Fatal("元数据 API 缺少鉴权")
		}
		return pikpakResponse(request, http.StatusOK, `{"id":"file","size":"42","web_content_link":"https://cdn.example/file?signature=test"}`), nil
	})
	source, err := (&Driver{}).GetDownloadURL(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "account-secret"}}, "file", 0)
	if err != nil {
		t.Fatal(err)
	}
	if source.Size != 42 || source.Headers["Authorization"] != "" || source.URL != "https://cdn.example/file?signature=test" {
		t.Fatalf("CDN 不应收到账号凭据: %+v", source)
	}
}
