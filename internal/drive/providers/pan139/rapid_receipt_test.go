package pan139

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestInstantUploadRequiresActualFileID(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous })
			requests := 0
			netx.TestTransportHook = pan139RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if !strings.HasSuffix(r.URL.Path, "/file/create") {
					t.Fatalf("秒传不应请求分片: %s", r.URL)
				}
				body := `{"success":true,"data":{"rapidUpload":true,"fileId":"file"}}`
				if missing {
					body = `{"success":true,"data":{"rapidUpload":true}}`
				}
				return pan139Response(r, 200, nil, body), nil
			})
			auth := encodeAuthorization("pc", "13800138000", fmt.Sprintf("token|a|b|%d", time.Now().Add(30*24*time.Hour).UnixMilli()))
			account := drive.Context{Token: &model.TokenInfo{AccessToken: auth, RefreshToken: mustJSON(map[string]string{"personalCloudHost": "https://personal.test", "account": "13800138000"})}}
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "file.txt", ParentFileID: RootID}}
			err := (&Driver{}).UploadOneFile(t.Context(), account, ui)
			if (err != nil) != missing || requests != 1 || !missing && (ui.Upload.FileID == "" || !ui.Info.IsMiaoChuan) {
				t.Fatalf("错误秒传结果: %+v %v %d", ui, err, requests)
			}
			result, err := (&Driver{}).RapidUploadByHash(t.Context(), account, drive.RapidUploadRequest{Method: "sha256", Hash: strings.Repeat("a", 64), FileName: "file.txt", ParentID: RootID, Size: 4})
			if (err != nil) != missing || !missing && (result == nil || !result.Reuse || result.FileID == "") {
				t.Fatalf("错误哈希秒传结果: %+v %v", result, err)
			}
		})
	}
}
