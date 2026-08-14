package pan189

import (
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestRegistration(t *testing.T) {
	reg, ok := drive.Get(model.ProviderPan189)
	if !ok {
		t.Fatal("pan189 not registered")
	}
	if reg.ID != model.ProviderPan189 {
		t.Fatalf("id = %q", reg.ID)
	}
	if reg.Meta.Label != "天翼云盘" {
		t.Fatalf("meta label = %q", reg.Meta.Label)
	}
	if reg.Auth == nil {
		t.Fatal("Auth must be wired for account+password login")
	}
	// login form must carry username/password
	keys := map[string]bool{}
	for _, f := range reg.Login.Fields {
		keys[f.Key] = true
	}
	if !keys["username"] || !keys["password"] {
		t.Fatalf("login fields missing username/password: %v", keys)
	}
	// factory builds a working driver
	d := reg.Factory()
	if d.ID() != model.ProviderPan189 {
		t.Fatalf("driver id = %q", d.ID())
	}
	if d.RootID() != PAN189Root {
		t.Fatalf("root = %q", d.RootID())
	}
}

func TestCapabilities(t *testing.T) {
	caps := drive.RegistryCaps(model.ProviderPan189)
	// legacy pan189 overrides
	if !caps.Copy {
		t.Error("copy must be enabled (legacy: copy: true)")
	}
	if !caps.RecycleBin {
		t.Error("recycleBin must be enabled")
	}
	if !caps.PermanentDelete {
		t.Error("permanentDelete must be enabled")
	}
	if caps.Search {
		t.Error("search must stay disabled (legacy: search: false)")
	}
	if caps.CreateShare {
		t.Error("createShare must stay disabled (legacy: createShare: false)")
	}
	if caps.TrashView {
		t.Error("trashView must stay disabled")
	}
	// md5 hashes both directions
	if len(caps.ProvideHashes) != 1 || caps.ProvideHashes[0] != "md5" {
		t.Errorf("provideHashes = %v", caps.ProvideHashes)
	}
	if len(caps.RapidUploadHashes) != 1 || caps.RapidUploadHashes[0] != "md5" {
		t.Errorf("rapidUploadHashes = %v", caps.RapidUploadHashes)
	}
	// standard file baseline retained
	if !caps.Upload || !caps.Download || !caps.CreateFolder || !caps.Rename || !caps.Move {
		t.Error("standard file capabilities must remain enabled")
	}
}

func TestExpireTimeFromURL(t *testing.T) {
	// AWS-style signed URL
	aws := "https://dl.example.com/x?X-Amz-Date=20240102T030405Z&X-Amz-Expires=3600&sig=abc"
	// base 2024-01-02 03:04:05 UTC = 1704164645000 ms
	if got := expireTimeFromURL(aws); got != 1704164645000+3600*1000 {
		t.Fatalf("aws expire = %d", got)
	}
	// x-oss-expires epoch seconds
	oss := "https://dl.example.com/x?x-oss-expires=1700000000"
	if got := expireTimeFromURL(oss); got != 1700000000*1000 {
		t.Fatalf("oss expire = %d", got)
	}
	// plain expire query (seconds timestamp)
	if got := expireTimeFromURL("https://dl.example.com/x?expire=1700000000&e=1"); got != 1700000000*1000 {
		t.Fatalf("expire = %d", got)
	}
	// RFC3339 expire value
	if got := expireTimeFromURL("https://dl.example.com/x?expires=2024-01-02T03:04:05Z"); got != 1704164645000 {
		t.Fatalf("rfc3339 expire = %d", got)
	}
	// no expiry params → 0
	if got := expireTimeFromURL("https://dl.example.com/x?foo=bar"); got != 0 {
		t.Fatalf("no-expire = %d", got)
	}
	if got := expireTimeFromURL(""); got != 0 {
		t.Fatalf("empty = %d", got)
	}
	// 189 download urls carry an expires-like query (present in practice)
	real := "https://d.pcs.189.cn/a?&expires=1710000000&x=1"
	if got := expireTimeFromURL(real); got != 1710000000*1000 {
		t.Fatalf("189-style expire = %d", got)
	}
}

func TestDriverImplementsInterface(t *testing.T) {
	var _ drive.Driver = (*Driver)(nil)
}

func TestGetInfoPseudoEntries(t *testing.T) {
	d := &Driver{}
	c := drive.Context{DriveID: "pan189:u"}
	for _, root := range []string{PAN189Root, "-11", "root", "/"} {
		info, err := d.GetInfo(t.Context(), c, root)
		if err != nil {
			t.Fatal(err)
		}
		f, ok := info.(model.File)
		if !ok || !f.IsDir || f.FileID != PAN189Root {
			t.Fatalf("GetInfo(%q) = %+v", root, info)
		}
	}
	info, err := d.GetInfo(t.Context(), c, "file-1")
	if err != nil {
		t.Fatal(err)
	}
	f := info.(model.File)
	if f.IsDir || f.FileID != "file-1" {
		t.Fatalf("GetInfo(file-1) = %+v", info)
	}
}
