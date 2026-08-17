package onedrive

import (
	"context"
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
