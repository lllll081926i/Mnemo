package transfer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mnemo-go/internal/store"
)

func TestScanUploadSourcesIsAtomicAndRejectsUnsafeInputs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "folder")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "中文.txt"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := scanUploadSources([]string{sub})
	if err != nil || len(out) != 1 || out[0].relative != "folder/中文.txt" || out[0].size != 2 {
		t.Fatalf("sources=%+v, error=%v", out, err)
	}
	_, err = scanUploadSources([]string{sub, filepath.Join(dir, "missing")})
	if err == nil {
		t.Fatal("missing path must fail the whole batch")
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	jobs, err := q.AddFilesChecked("account", "drive", "root", "rename", []string{sub, filepath.Join(dir, "missing")})
	if err == nil || len(jobs) != 0 || len(q.List()) != 0 {
		t.Fatalf("invalid batch enqueued a partial upload: jobs=%d all=%d err=%v", len(jobs), len(q.List()), err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(sub, "link.txt")
		if err := os.Symlink(filepath.Join(sub, "中文.txt"), link); err != nil {
			t.Fatal(err)
		}
		_, err = scanUploadSources([]string{sub})
		if err == nil || !strings.Contains(err.Error(), "符号链接") {
			t.Fatalf("symlink inside selection must fail: %v", err)
		}
	}
}

func TestScanUploadSourcesRejectsEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := scanUploadSources([]string{dir}); err == nil {
		t.Fatal("empty folder must not appear to upload successfully")
	}
}
