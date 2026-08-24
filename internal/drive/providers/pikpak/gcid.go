package pikpak

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"os"
	"strings"

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
func computeGCID(path string, ui *model.UploadingUI) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()
	chunkSize := gcidChunkSize(size)
	var chunkHashes []byte
	offset := int64(0)
	buf := make([]byte, chunkSize)
	var hashed int64
	for offset < size {
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return "", err
		}
		if n == 0 {
			break
		}
		sum := sha1.Sum(buf[:n])
		chunkHashes = append(chunkHashes, sum[:]...)
		offset += int64(n)
		hashed += int64(n)
		if ui != nil {
			ui.ReportUploadProgress(hashed, size)
		}
	}
	finalSum := sha1.Sum(chunkHashes)
	gcid := hex.EncodeToString(finalSum[:])
	// PikPak expects uppercase
	return strings.ToUpper(gcid), nil
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
