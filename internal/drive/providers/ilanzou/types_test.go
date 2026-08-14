package ilanzou

import (
	"testing"
	"time"
)

func TestToILanzouFolderId(t *testing.T) {
	cases := map[string]string{
		"":           "0",
		ILANZOU_ROOT: "0",
		"root":       "0",
		"/":          "0",
		"0":          "0",
		"123":        "123",
		"folder-456": "folder-456",
	}
	for in, want := range cases {
		if got := ToILanzouFolderId(in); got != want {
			t.Errorf("ToILanzouFolderId(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapILanzouItem(t *testing.T) {
	folder := mapILanzouItem(listItem{FileType: 2, FolderID: 9, FolderName: "dir"}, "ilanzou:u", "0")
	if !folder.IsDir {
		t.Error("folder should be a directory")
	}
	if folder.FileID != "9" {
		t.Errorf("folder file_id = %q", folder.FileID)
	}
	if folder.ParentFileID != ILANZOU_ROOT {
		t.Errorf("root listing parent = %q, want %q", folder.ParentFileID, ILANZOU_ROOT)
	}

	file := mapILanzouItem(listItem{FileType: 0, FileID: 8, FileName: "a.zip", FileSize: 2}, "ilanzou:u", "9")
	if file.IsDir {
		t.Error("file should not be a directory")
	}
	if file.FileID != "8" {
		t.Errorf("file file_id = %q", file.FileID)
	}
	// fileSize is in KB
	if file.Size != 2*1024 {
		t.Errorf("file size = %d, want %d", file.Size, 2*1024)
	}
	if file.Ext != "zip" {
		t.Errorf("file ext = %q, want zip", file.Ext)
	}

	// updTime parsing (local wall clock)
	item := listItem{FileType: 0, FileID: 7, FileName: "b.mp3", FileSize: 1, UpdTime: "2024-05-01 12:30:00"}
	mapped := mapILanzouItem(item, "ilanzou:u", "9")
	want, _ := time.ParseInLocation("2006-01-02 15:04:05", "2024-05-01 12:30:00", time.Local)
	if mapped.Time != want.Unix() {
		t.Errorf("time = %d, want %d", mapped.Time, want.Unix())
	}
}
