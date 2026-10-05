package guangya

import (
	"net/http"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestDownloadUsesGuangyaHeaders(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = guangyaRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/nd.bizuserres.s/v1/get_res_download_url" {
			t.Fatalf("意外路径: %s", request.URL.Path)
		}
		return guangyaResponse(request, http.StatusOK, `{"data":{"signedURL":"https://cdn.example/file?signature=test"}}`), nil
	})
	session := &Session{AccessToken: "access", DeviceID: "device", ClientID: clientID}
	source, err := (&Driver{}).GetDownloadURL(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "access", RefreshToken: mustJSON(session)}}, "file", 0)
	if err != nil {
		t.Fatal(err)
	}
	if source.Headers["Referer"] != "https://www.guangyapan.com/" || source.Headers["User-Agent"] != ua || source.Concurrency != 0 || !source.AllowLastModifiedFallback {
		t.Fatalf("光鸭下载策略错误: %+v", source)
	}
}
