package onedrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type onedriveRoundTripper func(*http.Request) (*http.Response, error)

func (f onedriveRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func onedriveResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestOneDriveTransferHashCapabilitiesAndResolver(t *testing.T) {
	caps := (&Driver{}).Capabilities()
	if strings.Join(caps.ProvideHashes, ",") != "sha1,quickxorhash" {
		t.Fatalf("ProvideHashes = %v, want [sha1 quickxorhash]", caps.ProvideHashes)
	}
	if len(caps.RapidUploadHashes) != 0 {
		t.Fatalf("RapidUploadHashes = %v, want none", caps.RapidUploadHashes)
	}

	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = onedriveRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Host != "graph.microsoft.com" || req.URL.Path != "/v1.0/me/drive/items/file-hash" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
		return onedriveResponse(req, http.StatusOK, `{"id":"file-hash","file":{"hashes":{"sha1Hash":"ABCDEF0123456789","quickXorHash":"quick-xor"}}}`), nil
	})

	c := drive.Context{Token: &model.TokenInfo{AccessToken: "access-token"}}
	sha1, err := (&Driver{}).ResolveTransferHash(context.Background(), c, "file-hash", "sha1", false)
	if err != nil || sha1 != "ABCDEF0123456789" {
		t.Fatalf("ResolveTransferHash(sha1) = %q, %v", sha1, err)
	}
	quickXor, err := (&Driver{}).ResolveTransferHash(context.Background(), c, "file-hash", "quickxorhash", false)
	if err != nil || quickXor != "quick-xor" {
		t.Fatalf("ResolveTransferHash(quickxorhash) = %q, %v", quickXor, err)
	}
	if hash, err := (&Driver{}).ResolveTransferHash(context.Background(), c, "file-hash", "md5", false); err != nil || hash != "" {
		t.Fatalf("ResolveTransferHash(md5) = %q, %v, want unsupported empty hash", hash, err)
	}
}

func TestCreateShareUsesMicrosoftGraphCreateLink(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	netx.TestTransportHook = onedriveRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "graph.microsoft.com" || req.URL.Path != "/v1.0/me/drive/items/file-1/createLink" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer access-token" {
			return nil, fmt.Errorf("authorization = %q", req.Header.Get("Authorization"))
		}
		var body struct {
			Type       string `json:"type"`
			Scope      string `json:"scope"`
			Password   string `json:"password"`
			Expiration string `json:"expirationDateTime"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		if body.Type != "view" || body.Scope != "anonymous" || body.Password != "p4ss" || body.Expiration != "2030-01-01T00:00:00Z" {
			return nil, fmt.Errorf("share body = %+v", body)
		}
		return onedriveResponse(req, http.StatusOK, `{"id":"permission-1","expirationDateTime":"2030-01-01T00:00:00Z","link":{"type":"view","scope":"anonymous","webUrl":"https://1drv.ms/u/share-1"}}`), nil
	})

	item, err := (&Driver{}).CreateShare(context.Background(), drive.Context{
		UserID: "onedrive:user", DriveID: "onedrive:user", Token: &model.TokenInfo{AccessToken: "access-token"},
	}, drive.ShareParams{FileIDs: []string{"file-1"}, Expiration: "2030-01-01T00:00:00Z", Password: "p4ss"})
	if err != nil {
		t.Fatalf("CreateShare() error = %v", err)
	}
	if item.ShareID != "permission-1" || item.ShareURL != "https://1drv.ms/u/share-1" || item.FileID != "file-1" || item.AccountID != "onedrive:user" {
		t.Fatalf("share = %+v", item)
	}
}

func TestOneDriveCancelShareDeletesCreatedPermission(t *testing.T) {
	old := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = old })
	netx.TestTransportHook = onedriveRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete || req.URL.Host != "graph.microsoft.com" || req.URL.Path != "/v1.0/me/drive/items/file-1/permissions/permission-1" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer access-token" {
			return nil, fmt.Errorf("authorization = %q", req.Header.Get("Authorization"))
		}
		return onedriveResponse(req, http.StatusNoContent, ""), nil
	})

	err := (&Driver{}).CancelShare(context.Background(), drive.Context{Token: &model.TokenInfo{AccessToken: "access-token"}}, model.ShareHistoryEntry{FileID: "file-1", ShareID: "permission-1"})
	if err != nil {
		t.Fatalf("CancelShare() error = %v", err)
	}
}

func TestOneDrivePresignedDownloadURLDoesNotCarryBearerToken(t *testing.T) {
	old := netx.TestTransportHook
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1.0/me/drive/items/file-1" {
			return onedriveResponse(r, http.StatusNotFound, `{}`), nil
		}
		if !strings.Contains(r.URL.RawQuery, "@microsoft.graph.downloadUrl") || !strings.Contains(r.URL.RawQuery, "@content.downloadUrl") {
			return nil, fmt.Errorf("detail request missed signed download URL fields: %q", r.URL.RawQuery)
		}
		return onedriveResponse(r, http.StatusOK, `{"id":"file-1","name":"movie.mp4","size":4,"@microsoft.graph.downloadUrl":"https://download.example/movie"}`), nil
	})
	t.Cleanup(func() { netx.TestTransportHook = old })

	dl, err := (&Driver{}).GetDownloadURL(context.Background(), drive.Context{
		DriveID: "onedrive-drive",
		Token:   &model.TokenInfo{AccessToken: "secret"},
	}, "file-1", 0)
	if err != nil {
		t.Fatalf("GetDownloadURL returned error: %v", err)
	}
	if dl.URL != "https://download.example/movie" || len(dl.Headers) != 0 {
		t.Fatalf("presigned download = %+v", dl)
	}
}

func TestOneDriveGetDownloadURLRefreshesMalformedAccessToken(t *testing.T) {
	old := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = old })
	graphCalls, refreshCalls := 0, 0
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "graph.microsoft.com":
			graphCalls++
			if r.Header.Get("Authorization") == "Bearer malformed" {
				return onedriveResponse(r, http.StatusUnauthorized, `{"error":{"code":"InvalidAuthenticationToken","message":"JWT is not well formed"}}`), nil
			}
			if r.Header.Get("Authorization") != "Bearer refreshed-token" {
				return nil, fmt.Errorf("unexpected Graph authorization %q", r.Header.Get("Authorization"))
			}
			return onedriveResponse(r, http.StatusOK, `{"id":"file-1","name":"movie.mp4","size":4,"@microsoft.graph.downloadUrl":"https://download.example/movie"}`), nil
		case "login.microsoftonline.com":
			refreshCalls++
			return onedriveResponse(r, http.StatusOK, `{"access_token":"refreshed-token","refresh_token":"refresh-new","expires_in":3600,"token_type":"Bearer"}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
	})

	token := &model.TokenInfo{AccessToken: "malformed", RefreshToken: "refresh-old", DeviceID: "client-id"}
	dl, err := (&Driver{}).GetDownloadURL(context.Background(), drive.Context{DriveID: "onedrive-drive", Token: token}, "file-1", 0)
	if err != nil {
		t.Fatalf("GetDownloadURL() error = %v", err)
	}
	if graphCalls != 2 || refreshCalls != 1 || dl.URL != "https://download.example/movie" || len(dl.Headers) != 0 {
		t.Fatalf("calls/download = graph:%d refresh:%d download:%+v", graphCalls, refreshCalls, dl)
	}
	if token.AccessToken != "refreshed-token" || token.RefreshToken != "refresh-new" {
		t.Fatalf("refreshed token = %+v", token)
	}
}

func TestOneDriveSmallUploadRefreshesTokenAndUsesKnownLength(t *testing.T) {
	path := t.TempDir() + "/payload.txt"
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = old })
	graphCalls, refreshCalls := 0, 0
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "graph.microsoft.com":
			graphCalls++
			if r.Method != http.MethodPut || r.ContentLength != 4 {
				return nil, fmt.Errorf("unexpected upload request %s length=%d", r.Method, r.ContentLength)
			}
			for _, encoding := range r.TransferEncoding {
				if strings.EqualFold(encoding, "chunked") {
					return nil, fmt.Errorf("small upload unexpectedly used chunked encoding")
				}
			}
			if r.Header.Get("Authorization") == "Bearer malformed" {
				return onedriveResponse(r, http.StatusUnauthorized, `{"error":{"code":"InvalidAuthenticationToken","message":"JWT is not well formed"}}`), nil
			}
			if r.Header.Get("Authorization") != "Bearer refreshed-token" {
				return nil, fmt.Errorf("unexpected upload authorization %q", r.Header.Get("Authorization"))
			}
			return onedriveResponse(r, http.StatusCreated, `{"id":"uploaded-file"}`), nil
		case "login.microsoftonline.com":
			refreshCalls++
			return onedriveResponse(r, http.StatusOK, `{"access_token":"refreshed-token","refresh_token":"refresh-new","expires_in":3600,"token_type":"Bearer"}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
	})

	token := &model.TokenInfo{AccessToken: "malformed", RefreshToken: "refresh-old", DeviceID: "client-id"}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "payload.txt"}}
	if err := (&Driver{}).UploadOneFile(context.Background(), drive.Context{DriveID: "onedrive-drive", Token: token}, ui); err != nil {
		t.Fatalf("UploadOneFile() error = %v", err)
	}
	if graphCalls != 2 || refreshCalls != 1 || ui.Upload.FileID != "uploaded-file" || token.AccessToken != "refreshed-token" {
		t.Fatalf("calls/upload/token = graph:%d refresh:%d upload:%+v token:%+v", graphCalls, refreshCalls, ui.Upload, token)
	}
}

func TestOneDriveContentFallbackKeepsBearerToken(t *testing.T) {
	old := netx.TestTransportHook
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1.0/me/drive/items/file-1" {
			return onedriveResponse(r, http.StatusNotFound, `{}`), nil
		}
		return onedriveResponse(r, http.StatusOK, `{"id":"file-1","name":"movie.mp4","size":4}`), nil
	})
	t.Cleanup(func() { netx.TestTransportHook = old })

	dl, err := (&Driver{}).GetDownloadURL(context.Background(), drive.Context{
		DriveID: "onedrive-drive",
		Token:   &model.TokenInfo{AccessToken: "secret"},
	}, "file-1", 0)
	if err != nil {
		t.Fatalf("GetDownloadURL returned error: %v", err)
	}
	if dl.Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("content fallback headers = %#v", dl.Headers)
	}
}

func TestOneDriveListRefreshesInvalidAccessTokenWithoutSurfacingInitial401(t *testing.T) {
	old := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = old })

	graphCalls := 0
	refreshCalls := 0
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "graph.microsoft.com":
			graphCalls++
			if r.Header.Get("Authorization") == "Bearer stale-token" {
				return onedriveResponse(r, http.StatusUnauthorized, `{"error":{"code":"InvalidAuthenticationToken","message":"JWT is not well formed"}}`), nil
			}
			if r.Header.Get("Authorization") != "Bearer refreshed-token" {
				return nil, fmt.Errorf("unexpected graph authorization %q", r.Header.Get("Authorization"))
			}
			return onedriveResponse(r, http.StatusOK, `{"value":[{"id":"file-1","name":"one.txt","size":1,"file":{}}]}`), nil
		case "login.microsoftonline.com":
			refreshCalls++
			if r.Method != http.MethodPost || r.URL.String() != msTokenURL {
				return nil, fmt.Errorf("unexpected refresh request %s %s", r.Method, r.URL)
			}
			return onedriveResponse(r, http.StatusOK, `{"access_token":"refreshed-token","refresh_token":"refresh-new","expires_in":3600,"token_type":"Bearer"}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
	})

	token := &model.TokenInfo{AccessToken: "stale-token", RefreshToken: "refresh-old", DeviceID: "client-id"}
	items, err := (&Driver{}).List(context.Background(), drive.Context{DriveID: "onedrive-drive", Token: token}, RootID, nil)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if graphCalls != 2 || refreshCalls != 1 || len(items) != 1 || items[0].FileID != "file-1" {
		t.Fatalf("calls/items = graph:%d refresh:%d items:%+v", graphCalls, refreshCalls, items)
	}
	if token.AccessToken != "refreshed-token" || token.RefreshToken != "refresh-new" {
		t.Fatalf("rotated token = %+v", token)
	}
}

func TestGraphAuthenticationFailureUsesStructuredError(t *testing.T) {
	authErr := graphError([]byte(`{"error":{"code":"InvalidAuthenticationToken","message":"JWT is not well formed"}}`), http.StatusUnauthorized)
	if !isGraphAuthenticationFailure(fmt.Errorf("list root: %w", authErr)) {
		t.Fatal("structured InvalidAuthenticationToken should allow one refresh retry")
	}

	accessErr := graphError([]byte(`{"error":{"code":"AccessDenied","message":"policy blocked"}}`), http.StatusUnauthorized)
	if isGraphAuthenticationFailure(accessErr) {
		t.Fatal("unrelated Graph 401 must not refresh the token")
	}
	if isGraphAuthenticationFailure(fmt.Errorf("onedrive: http 401: InvalidAuthenticationToken")) {
		t.Fatal("formatted error text alone must not trigger a refresh retry")
	}
}

func TestRefreshOneDriveUsesConfiguredMatchingClientSecret(t *testing.T) {
	oldTransport := netx.TestTransportHook
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "login.microsoftonline.com" || r.Method != http.MethodPost {
			return nil, fmt.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		if form.Get("client_id") != "configured-client" || form.Get("client_secret") != "configured-secret" {
			return nil, fmt.Errorf("refresh credentials = client_id:%q client_secret:%q", form.Get("client_id"), form.Get("client_secret"))
		}
		return onedriveResponse(r, http.StatusOK, `{"access_token":"fresh","expires_in":3600,"token_type":"Bearer"}`), nil
	})
	drive.SetSecretResolver(func(key string) string {
		switch key {
		case "onedrive_client_id":
			return "configured-client"
		case "onedrive_client_secret":
			return "configured-secret"
		default:
			return ""
		}
	})
	t.Cleanup(func() {
		netx.TestTransportHook = oldTransport
		drive.SetSecretResolver(nil)
	})

	token := &model.TokenInfo{AccessToken: "malformed", RefreshToken: "refresh", DeviceID: "configured-client"}
	if err := refreshOneDriveAccessToken(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "fresh" || token.DeviceID != "configured-client" {
		t.Fatalf("refreshed token = %+v", token)
	}

	if _, secret := resolveCredentials("different-client", "", "configured-client", "configured-secret"); secret != "" {
		t.Fatalf("unrelated client must not receive configured secret: %q", secret)
	}
}
