package pikpak

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/model"
)

func TestComputeGCIDMatchesChunkFingerprint(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("PikPak 上传指纹"), make([]byte, 262145)} {
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := computeGCID(t.Context(), path, nil)
		if err != nil || got != gcidFromBytes(data) || len(got) != 40 {
			t.Fatalf("GCID = %q, %v，期望 %q", got, err, gcidFromBytes(data))
		}
		if len(data) > 0 && len(data) <= 262144 {
			chunk := sha1.Sum(data)
			digest := sha1.Sum(chunk[:])
			if got != hex.EncodeToString(digest[:]) {
				t.Fatal("GCID 与独立 SHA1 计算不一致")
			}
		}
	}
}

func TestComputeGCIDStopsDuringHashing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, make([]byte, 3*262144), 0600); err != nil {
		t.Fatal(err)
	}
	stopped, samples := false, 0
	ui := &model.UploadingUI{}
	ui.ConfigureUploadRuntime(nil, func() bool { return stopped })
	ui.ConfigureUploadStage(func(string, int) { samples++; stopped = true })
	_, err := computeGCID(t.Context(), path, ui)
	if err == nil || samples != 1 {
		t.Fatalf("哈希取消后仍继续读取：samples=%d, error=%v", samples, err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("取消未返回明确错误")
	}
}
