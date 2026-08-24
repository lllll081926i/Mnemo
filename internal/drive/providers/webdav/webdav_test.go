package webdav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestUploadStreamRenamesConflictAndUploadsExactBody(t *testing.T) {
	var mu sync.Mutex
	var putPath, putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			if r.URL.Path != "/movie.txt" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/movie.txt</D:href><D:propstat><D:prop><D:getcontentlength>3</D:getcontentlength><D:resourcetype/></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			putPath, putBody = r.URL.Path, string(body)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	c := drive.Context{Token: &model.TokenInfo{Conn: &model.ConnConfig{
		Endpoint: server.URL, AllowPrivateNetwork: true,
	}}}
	if err := (&Driver{}).UploadStream(context.Background(), c, "/", "movie.txt", 7, strings.NewReader("payload")); err != nil {
		t.Fatalf("UploadStream: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if putPath != "/movie (1).txt" || putBody != "payload" {
		t.Fatalf("PUT path/body = %q/%q, want renamed path and payload", putPath, putBody)
	}
}

func TestUploadStreamRejectsShortBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
	}))
	t.Cleanup(server.Close)
	c := drive.Context{Token: &model.TokenInfo{Conn: &model.ConnConfig{
		Endpoint: server.URL, AllowPrivateNetwork: true,
	}}}
	err := (&Driver{}).UploadStream(context.Background(), c, "/", "short.bin", 8, strings.NewReader("tiny"))
	if err == nil {
		t.Fatal("UploadStream accepted a short request body")
	}
}

func TestWebDAVDoesNotAdvertiseHashRapidUpload(t *testing.T) {
	caps := (&Driver{}).Capabilities()
	if len(caps.ProvideHashes) != 0 || len(caps.RapidUploadHashes) != 0 {
		t.Fatalf("generic WebDAV must not advertise ETag as a content hash: %+v", caps)
	}
}

var _ drive.StreamUploader = (*Driver)(nil)
