package guangya

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestGuangyaInstantUploadRequiresConfirmedIdentity(t *testing.T) {
	for _, mode := range []string{"direct", "task", "missing", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous })
			polls := 0
			netx.TestTransportHook = guangyaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{"code":156,"data":{"taskId":"task"}}`
				if strings.HasSuffix(r.URL.Path, "get_info_by_task_id") {
					polls++
					body = `{"code":0,"data":{"fileId":"file"}}`
					if mode == "rejected" {
						body = `{"code":999,"msg":"拒绝","data":{"fileId":"file"}}`
					}
				} else if mode == "direct" {
					body = `{"code":156,"data":{"fileId":"file"}}`
				} else if mode == "missing" {
					body = `{"code":156,"data":{}}`
				}
				return guangyaResponse(r, 200, body), nil
			})
			ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "file.txt"}}
			err := (&Driver{}).UploadOneFile(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "access", RefreshToken: mustJSON(&Session{AccessToken: "access"})}}, ui)
			failed := mode == "missing" || mode == "rejected"
			if (err != nil) != failed || !failed && (ui.Upload.FileID != "file" || !ui.Info.IsMiaoChuan || ui.Upload.DownProcess != 100) {
				t.Fatalf("错误秒传回执: %+v %v", ui, err)
			}
			if mode == "direct" && polls != 0 {
				t.Fatal("直接回执不应继续轮询")
			}
		})
	}
}
