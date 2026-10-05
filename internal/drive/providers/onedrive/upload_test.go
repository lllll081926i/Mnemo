package onedrive

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestSessionUploadRequiresRemoteCompletion(t *testing.T) {
	for _, result := range []struct {
		status int
		body   string
		ok     bool
	}{
		{202, `{"nextExpectedRanges":["3-"]}`, false},
		{201, `{}`, false},
		{201, `{"id":"remote-file"}`, true},
	} {
		t.Run(http.StatusText(result.status)+result.body, func(t *testing.T) {
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous })
			netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
				switch request.Method {
				case http.MethodPost:
					return onedriveResponse(request, 200, `{"uploadUrl":"https://upload.example/session"}`), nil
				case http.MethodGet:
					return onedriveResponse(request, 200, `{"nextExpectedRanges":["0-"]}`), nil
				case http.MethodPut:
					return onedriveResponse(request, result.status, result.body), nil
				}
				t.Fatalf("意外请求 %s", request.Method)
				return nil, nil
			})
			path := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			ui := &model.UploadingUI{}
			err = newClient("token").sessionUpload(context.Background(), drive.Context{}, file, "parent", "file", ui, "replace")
			if (err == nil) != result.ok || result.ok && ui.Upload.FileID != "remote-file" {
				t.Fatalf("未核实服务端提交：fileID=%q, error=%v", ui.Upload.FileID, err)
			}
		})
	}
}
