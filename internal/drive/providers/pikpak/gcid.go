package pikpak

import (
	"context"
	"crypto/sha1"
	"encoding/hex"

	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

// GCID chunk sizes (mirrors reference).
func gcidChunkSize(fileSize int64) int64 {
	switch {
	case fileSize <= 0x8000000:
		return 262144
	case fileSize <= 0x10000000:
		return 524288
	case fileSize <= 0x20000000:
		return 1048576
	default:
		return 2097152
	}
}

// computeGCID computes the PikPak GCID for a local file while reporting
// progress via ui.Upload.
func computeGCID(ctx context.Context, path string, ui *model.UploadingUI) (string, error) {
	return driveutil.ComputeGCID(ctx, path, ui)
}

// gcidFromBytes computes GCID from a byte slice (for testing).
func gcidFromBytes(data []byte) string {
	chunkSize := int(gcidChunkSize(int64(len(data))))
	var chunkHashes []byte
	for offset := 0; offset < len(data); offset += chunkSize {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		sum := sha1.Sum(data[offset:end])
		chunkHashes = append(chunkHashes, sum[:]...)
	}
	sum := sha1.Sum(chunkHashes)
	return hex.EncodeToString(sum[:])
}
