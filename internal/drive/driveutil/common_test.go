package driveutil

import (
	"testing"
)

func TestExt(t *testing.T) {
	if Ext("file.txt") != "txt" {
		t.Fatal("txt")
	}
	if Ext("archive.tar.gz") != "gz" {
		t.Fatal("gz")
	}
	if Ext("noext") != "" {
		t.Fatal("noext")
	}
	if Ext(".hidden") != "hidden" {
		t.Fatal("hidden")
	}
}

func TestGuessCategory(t *testing.T) {
	if GuessCategory("video.mp4") != CatVideo {
		t.Fatal("mp4")
	}
	if GuessCategory("audio.mp3") != CatAudio {
		t.Fatal("mp3")
	}
	if GuessCategory("image.jpg") != CatImage {
		t.Fatal("jpg")
	}
	if GuessCategory("doc.pdf") != CatDoc {
		t.Fatal("pdf")
	}
	if GuessCategory("archive.zip") != CatArchive {
		t.Fatal("zip")
	}
	if GuessCategory("code.go") != CatText {
		t.Fatal("go")
	}
	if GuessCategory("unknown.xyz") != CatOther {
		t.Fatal("xyz")
	}
}

func TestNewFile(t *testing.T) {
	f := NewFile("d1", "f1", "p1", "test.txt", false, 100, 1700000000)
	if f.DriveID != "d1" {
		t.Fatal("drive_id")
	}
	if f.FileID != "f1" {
		t.Fatal("file_id")
	}
	if f.Name != "test.txt" {
		t.Fatal("name")
	}
	if f.Ext != "txt" {
		t.Fatal("ext")
	}
	if f.Category != CatText {
		t.Fatal("category")
	}
	if f.Size != 100 {
		t.Fatal("size")
	}
	if f.IsDir {
		t.Fatal("isdir")
	}
	if f.SizeStr == "" {
		t.Fatal("sizeStr")
	}
}

func TestNewFileDir(t *testing.T) {
	f := NewFile("d1", "f1", "p1", "folder", true, 0, 0)
	if !f.IsDir {
		t.Fatal("not dir")
	}
	if f.Category != CatOther {
		t.Fatal("dir cat")
	}
}

func TestJoinPath(t *testing.T) {
	if JoinPath("/", "sub") != "/sub" {
		t.Fatal("root")
	}
	if JoinPath("/parent", "sub") != "/parent/sub" {
		t.Fatal("parent")
	}
	if JoinPath("/parent/", "sub") != "/parent/sub" {
		t.Fatal("trailing")
	}
}
