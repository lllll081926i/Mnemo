package s3

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestOverwriteVirtualDirectoryNeverDeletesExistingObjects(t *testing.T) {
	previous := TransportOverride
	t.Cleanup(func() { TransportOverride = previous })
	methods := make([]string, 0)
	TransportOverride = uploadRoundTripper(func(req *http.Request) (*http.Response, error) {
		methods = append(methods, req.Method)
		switch {
		case req.Method == http.MethodHead:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(`<Error><Code>NoSuchKey</Code></Error>`)),
				Request: req,
			}, nil
		case req.Method == http.MethodGet && req.URL.Query().Get("list-type") == "2":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(`<ListBucketResult><Name>bucket</Name><Contents><Key>mount/root/occupied/old.txt</Key></Contents><IsTruncated>false</IsTruncated></ListBucketResult>`)),
				Request: req,
			}, nil
		default:
			t.Errorf("unsafe remote request: %s %s", req.Method, req.URL)
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader("unexpected")),
				Request: req,
			}, nil
		}
	})
	account := drive.Context{UserID: "account", DriveID: "drive", Token: &model.TokenInfo{Conn: testS3Config()}}
	ui := &model.UploadingUI{Info: model.UploadInfo{
		LocalFilePath: "not-opened-on-conflict",
		Name: "occupied",
		ParentFileID: "/",
		ConflictPolicy: "overwrite",
	}}
	err := (&Driver{}).UploadOneFile(context.Background(), account, ui)
	if err == nil {
		t.Fatal("overwriting a nonempty virtual directory should be rejected")
	}
	for _, method := range methods {
		if method == http.MethodDelete || method == http.MethodPut || method == http.MethodPost {
			t.Fatalf("upload performed a destructive action despite conflict: %v", methods)
		}
	}
}
