package pan123

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/model"
)

func TestUploadSliceRejectsTruncatedSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := readSlice(file, 0, 5); err == nil {
		t.Fatal("上传分片接受了短文件")
	}
}

func TestUploadHashCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, make([]byte, 3*256*1024), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ui := &model.UploadingUI{}
	samples := 0
	ui.ConfigureUploadStage(func(string, int) { samples++; cancel() })
	if _, err := fileMD5(ctx, path, ui); !errors.Is(err, context.Canceled) || samples != 1 {
		t.Fatalf("取消后继续计算 MD5：%d, %v", samples, err)
	}
}
