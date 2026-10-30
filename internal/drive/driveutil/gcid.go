package driveutil

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"

	"mnemo-go/internal/model"
)

// ComputeGCID 计算迅雷与 PikPak 使用的分块 SHA1 指纹。
func ComputeGCID(ctx context.Context, path string, ui *model.UploadingUI) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	block := int64(256 << 10)
	for info.Size() > block*512 && block < 2<<20 {
		block *= 2
	}
	buf := make([]byte, block)
	combined := sha1.New()
	for offset := int64(0); offset < info.Size(); {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if ui != nil && ui.IsUploadStopRequested() {
			return "", context.Canceled
		}
		n, err := io.ReadFull(f, buf[:min(block, info.Size()-offset)])
		if err != nil {
			return "", err
		}
		sum := sha1.Sum(buf[:n])
		_, _ = combined.Write(sum[:])
		offset += int64(n)
		if ui != nil {
			ui.ReportHashProgress(offset, info.Size())
		}
	}
	return hex.EncodeToString(combined.Sum(nil)), nil
}
