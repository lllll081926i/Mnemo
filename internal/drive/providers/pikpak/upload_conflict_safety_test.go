package pikpak

import (
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestPikPakOverwriteConflictDoesNotTrashExistingFile(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	requests := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodGet || req.URL.Path != "/drive/v1/files" {
			t.Errorf("dangerous operation before overwrite confirmation: %s %s", req.Method, req.URL)
			return pikpakResponse(req, 500, `{"error":"unexpected"}`), nil
		}
		return pikpakResponse(req, 200, `{"files":[{"id":"old-file","name":"protected.mkv","kind":"drive#file"}]}`), nil
	})
	cl := newClient("access", "device", "account")
	ui := &model.UploadingUI{Info: model.UploadInfo{
		Name: "protected.mkv",
		ParentFileID: RootID,
		ConflictPolicy: "overwrite",
	}}
	proceed, err := (&Driver{}).prepareUploadTarget(t.Context(), cl, ui)
	if proceed || err == nil || requests != 1 {
		t.Fatalf("unsafe overwrite: proceed=%v err=%v requests=%d", proceed, err, requests)
	}
	if !strings.Contains(err.Error(), "安全覆盖") {
		t.Fatalf("misleading conflict error: %v", err)
	}
}

func TestPikPakConflictRenameUsesOriginalName(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	requests := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodGet || req.URL.Path != "/drive/v1/files" {
			t.Errorf("unexpected operation: %s %s", req.Method, req.URL)
		}
		return pikpakResponse(req, 200, `{"files":[{"id":"original","name":"video.mkv"},{"id":"copy-1","name":"video (1).mkv"}]}`), nil
	})
	cl := newClient("access", "device", "account")
	ui := &model.UploadingUI{Info: model.UploadInfo{Name: "video.mkv", ParentFileID: RootID, ConflictPolicy: "rename"}}
	ok, err := (&Driver{}).prepareUploadTarget(t.Context(), cl, ui)
	if !ok || err != nil || ui.Info.Name != "video (2).mkv" || requests != 3 {
		t.Fatalf("conflict rename: name=%q ok=%v err=%v requests=%d", ui.Info.Name, ok, err, requests)
	}
}
