package model

import (
	"sync"
	"testing"
)

func TestUploadingUIRuntimeProgress(t *testing.T) {
	var mu sync.Mutex
	var gotDone, gotPercent int
	ui := &UploadingUI{Info: UploadInfo{Size: 100}}
	ui.ConfigureUploadRuntime(func(done int64, percent int) {
		mu.Lock()
		gotDone, gotPercent = int(done), percent
		mu.Unlock()
	}, func() bool { return false })
	ui.ReportUploadProgress(160, 100)
	mu.Lock()
	defer mu.Unlock()
	if gotDone != 100 || gotPercent != 100 || ui.Upload.DownSize != 100 || ui.Upload.DownProcess != 100 {
		t.Fatalf("progress = done:%d percent:%d state:%+v", gotDone, gotPercent, ui.Upload)
	}
	if ui.IsUploadStopRequested() {
		t.Fatal("stop callback should report false")
	}
}

func TestUploadingUIRuntimeProgressZeroByte(t *testing.T) {
	ui := &UploadingUI{}
	ui.ReportUploadProgress(0, 0)
	if ui.Upload.DownProcess != 100 {
		t.Fatalf("zero-byte progress = %d, want 100", ui.Upload.DownProcess)
	}
}

func TestFormatBytes(t *testing.T) {
	if FormatBytes(0) != "0 B" {
		t.Fatal("0")
	}
	if FormatBytes(1023) != "1023 B" {
		t.Fatal("1023")
	}
	if FormatBytes(1024) != "1.0 KB" {
		t.Fatal("1KB")
	}
	if FormatBytes(1536) != "1.5 KB" {
		t.Fatal("1.5KB")
	}
	if FormatBytes(1048576) != "1.0 MB" {
		t.Fatal("1MB")
	}
	if FormatBytes(1073741824) != "1.0 GB" {
		t.Fatal("1GB")
	}
}

func TestBuildUserID(t *testing.T) {
	if BuildUserID("pikpak", "123") != "pikpak_123" {
		t.Fatal("pikpak")
	}
	if BuildUserID("webdav", "example.com") != "webdav:example.com" {
		t.Fatal("webdav")
	}
	if BuildUserID("s3", "bucket") != "s3:bucket" {
		t.Fatal("s3")
	}
	if BuildUserID("pikpak", "pikpak_123") != "pikpak_123" {
		t.Fatal("already prefixed")
	}
}

func TestStripUserID(t *testing.T) {
	if StripUserID("pikpak", "pikpak_123") != "123" {
		t.Fatal("pikpak")
	}
	if StripUserID("webdav", "webdav:example.com") != "example.com" {
		t.Fatal("webdav")
	}
	if StripUserID("pikpak", "123") != "123" {
		t.Fatal("no prefix")
	}
}

func TestBuildDriveID(t *testing.T) {
	if BuildDriveID("pikpak", "123") != "pikpak:123" {
		t.Fatal("pikpak")
	}
	if BuildDriveID("pikpak", "pikpak:123") != "pikpak:123" {
		t.Fatal("already")
	}
}

func TestAllProviders(t *testing.T) {
	p := AllProviders()
	if len(p) <= 10 {
		t.Fatal("too few providers:", len(p))
	}
	found := map[string]bool{}
	for _, v := range p {
		if found[v] {
			t.Fatal("duplicate:", v)
		}
		found[v] = true
	}
	if !found["pikpak"] {
		t.Fatal("missing pikpak")
	}
}

func TestFormatSpeed(t *testing.T) {
	if FormatSpeed(0) != "0 B/s" {
		t.Fatal("0")
	}
	if FormatSpeed(1024) != "1.0 KB/s" {
		t.Fatal("1KB/s")
	}
	if FormatSpeed(10485760) != "10.0 MB/s" {
		t.Fatal("10MB/s")
	}
}
