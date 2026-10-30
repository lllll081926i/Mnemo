package pikpak

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestPikPakInstantUploadRequiresCompletePhase(t *testing.T) {
	for _, phase := range []string{"PHASE_TYPE_COMPLETE", "PHASE_TYPE_RUNNING"} {
		t.Run(phase, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "file.txt")
			if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous })
			netx.TestTransportHook = pikpakRoundTripper(func(r *http.Request) (*http.Response, error) {
				body := `{"files":[]}`
				if r.Method == http.MethodPost {
					body = `{"file":{"id":"file"}}`
				} else if r.URL.Path == "/drive/v1/files/file" {
					body = `{"id":"file","phase":"` + phase + `"}`
				}
				return pikpakResponse(r, 200, body), nil
			})
			ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "file.txt", ParentFileID: RootID}}
			err := (&Driver{}).UploadOneFile(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "access", DeviceID: "device", ProviderAccountID: "account"}}, ui)
			if phase == "PHASE_TYPE_COMPLETE" {
				if err != nil || !ui.Upload.IsCompleted || !ui.Info.IsMiaoChuan {
					t.Fatalf("秒传未确认: %+v %v", ui, err)
				}
			} else if err == nil {
				t.Fatal("待上传对象被误判为秒传完成")
			}
		})
	}
}
