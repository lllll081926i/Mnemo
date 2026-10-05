package pan189

import (
	"bytes"
	"testing"

	"mnemo-go/internal/model"
)

func TestUploadHashRejectsShortRead(t *testing.T) {
	if _, _, _, err := fileMD5Of(t.Context(), bytes.NewReader([]byte("abc")), 5, 5, 1, nil); err == nil {
		t.Fatal("上传哈希把短文件填零后视为完整文件")
	}
}

func TestUploadHashStopsBetweenParts(t *testing.T) {
	stopped, samples := false, 0
	ui := &model.UploadingUI{}
	ui.ConfigureUploadRuntime(nil, func() bool { return stopped })
	ui.ConfigureUploadStage(func(string, int) { samples++; stopped = true })
	if _, _, _, err := fileMD5Of(t.Context(), bytes.NewReader([]byte("abcdef")), 6, 2, 3, ui); err == nil || samples != 1 {
		t.Fatalf("哈希取消未停止：samples=%d, error=%v", samples, err)
	}
}
