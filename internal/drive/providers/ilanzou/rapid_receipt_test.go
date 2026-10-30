package ilanzou

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestLocalInstantUploadAndMissingIdentity(t *testing.T) {
	withNoThrottle(t)
	previous := ILANZOU_CONF.Base
	t.Cleanup(func() { ILANZOU_CONF.Base = previous })
	missing := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missing {
			_, _ = w.Write([]byte(`{"code":200}`))
		} else {
			_, _ = w.Write([]byte(`{"code":200,"fileId":"rapid-1"}`))
		}
	}))
	defer server.Close()
	ILANZOU_CONF.Base = server.URL
	account := drive.Context{Token: &model.TokenInfo{AccessToken: "api-token", DeviceID: "uuid-42"}}
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "file.txt", ParentFileID: (&Driver{}).RootID()}}
	if err := (&Driver{}).UploadOneFile(t.Context(), account, ui); err != nil || ui.Upload.FileID != "rapid-1" || !ui.Info.IsMiaoChuan {
		t.Fatalf("秒传未完成: %+v %v", ui, err)
	}
	missing = true
	result, err := (&Driver{}).RapidUploadByHash(t.Context(), account, drive.RapidUploadRequest{Method: "md5", Hash: "8d777f385d3dfec8815d20f7496026dc", Size: 4, FileName: "file.txt", ParentID: (&Driver{}).RootID()})
	if err == nil || result != nil && result.Reuse {
		t.Fatalf("缺少文件 ID 不能回退创建或报告成功: %+v %v", result, err)
	}
}
