package onedrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func TestOneDrivePresignedDownloadURLDoesNotCarryBearerToken(t *testing.T) {
	old := netx.TestTransportHook
	netx.TestTransportHook = onedriveRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1.0/me/drive/items/file-1" {
			return onedriveResponse(r, http.StatusNotFound, `{}`), nil
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
