package lanzou

import (
	"testing"
)

func TestToLanzouFolderId(t *testing.T) {
	cases := map[string]string{
		"":           "-1",
		LANZOU_ROOT:  "-1",
		"root":       "-1",
		"/":          "-1",
		"-1":         "-1",
		"123":        "123",
		"folder-456": "folder-456",
	}
	for in, want := range cases {
		if got := ToLanzouFolderId(in); got != want {
			t.Errorf("ToLanzouFolderId(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSizeToBytes(t *testing.T) {
	cases := map[string]int64{
		"":      0,
		"1.5 M": int64(1.5 * 1024 * 1024),
		"10K":   10 * 1024,
		"2B":    2,
		"1G":    1024 * 1024 * 1024,
		"3":     3,
	}
	for in, want := range cases {
		if got := parseSizeToBytes(in); got != want {
			t.Errorf("parseSizeToBytes(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestMapLanzouItem(t *testing.T) {
	folder := mapLanzouItem(LanzouItem{FolID: "9", Name: "dir"}, "lanzou:u", "-1")
	if !folder.IsDir {
		t.Error("folder should be a directory")
	}
	if folder.FileID != "9" {
		t.Errorf("folder file_id = %q", folder.FileID)
	}
	if folder.ParentFileID != LANZOU_ROOT {
		t.Errorf("root listing parent = %q, want %q", folder.ParentFileID, LANZOU_ROOT)
	}
	if folder.Icon != "iconfile-folder" {
		t.Errorf("folder icon = %q", folder.Icon)
	}

	file := mapLanzouItem(LanzouItem{ID: "8", NameAll: "a.zip", Size: "2M"}, "lanzou:u", "9")
	if file.IsDir {
		t.Error("file should not be a directory")
	}
	if file.FileID != "8" {
		t.Errorf("file file_id = %q", file.FileID)
	}
	if file.Ext != "zip" {
		t.Errorf("file ext = %q, want zip", file.Ext)
	}
	if file.Size != 2*1024*1024 {
		t.Errorf("file size = %d, want %d", file.Size, 2*1024*1024)
	}
	if file.ParentFileID != "9" {
		t.Errorf("file parent = %q, want 9", file.ParentFileID)
	}
}
