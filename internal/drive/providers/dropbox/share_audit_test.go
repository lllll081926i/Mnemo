package dropbox

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestShareCreationDoesNotHidePermissionFailureBehindExistingLink(t *testing.T) {
	calls := 0
	withDropboxTransport(t, dropboxRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		if dropboxAPIPath(request) != "/sharing/create_shared_link_with_settings" {
			t.Fatalf("权限错误不应读取或改写旧分享: %s", request.URL)
		}
		return dropboxResponse(request, http.StatusForbidden, `{"error_summary":"access_denied/"}`), nil
	}))
	share, err := newClient("access").CreateSharedLink(t.Context(), "/file", "", "")
	if err == nil || share != nil || calls != 1 {
		t.Fatalf("创建失败被掩盖: share=%+v, err=%v, calls=%d", share, err, calls)
	}
}

func TestExistingLinkAppliesPublicAndNoExpirationSettings(t *testing.T) {
	modified := false
	withDropboxTransport(t, dropboxRoundTripper(func(request *http.Request) (*http.Response, error) {
		switch dropboxAPIPath(request) {
		case "/sharing/create_shared_link_with_settings":
			return dropboxResponse(request, http.StatusConflict, `{"error_summary":"shared_link_already_exists/"}`), nil
		case "/sharing/list_shared_links":
			return dropboxResponse(request, http.StatusOK, `{"links":[{"url":"https://www.dropbox.com/s/old"}],"has_more":false}`), nil
		case "/sharing/modify_shared_link_settings":
			modified = true
			var body struct {
				Settings map[string]any `json:"settings"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Settings["remove_expiration"] != true || body.Settings["requested_visibility"] != "public" {
				t.Fatalf("沿用了旧链接的设置: %+v", body)
			}
			return dropboxResponse(request, http.StatusOK, `{"url":"https://www.dropbox.com/s/old"}`), nil
		default:
			t.Fatalf("未知请求: %s", request.URL)
			return nil, nil
		}
	}))
	share, err := newClient("access").CreateSharedLink(t.Context(), "/file", "", "")
	if err != nil || share == nil || !modified {
		t.Fatalf("未应用用户设置: %v", err)
	}
}
