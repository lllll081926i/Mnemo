package dlengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// commitPart replaces the destination on every platform. Windows refuses to
// rename over an existing file, so move the old destination aside first and
// restore it if the commit fails.
func commitPart(partPath, localPath string) error {
	if err := os.Rename(partPath, localPath); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	if _, err := os.Stat(localPath); err != nil {
		return err
	}
	backup, err := os.CreateTemp(filepath.Dir(localPath), ".mnemo-replace-*")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if err := os.Rename(localPath, backupPath); err != nil {
		return err
	}
	if err := os.Rename(partPath, localPath); err != nil {
		_ = os.Rename(backupPath, localPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func ensureFile(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if size < 0 {
		return errors.New("dlengine: negative file size")
	}
	cur, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if cur != size {
		if err := f.Truncate(size); err != nil {
			return err
		}
	}
	if cur < size {
		if _, err := f.Seek(size-1, io.SeekStart); err != nil {
			return err
		}
		if _, err := f.Write([]byte{0}); err != nil {
			return err
		}
	}
	return nil
}

func chunkLen(st *state, idx int) int64 {
	start := int64(idx) * st.Chunk
	if start+st.Chunk > st.Total {
		return st.Total - start
	}
	return st.Chunk
}

func percent(done, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(done * 100 / total)
}

func persistState(path string, st *state) error {
	safeState := *st
	if safeState.URLHash == "" && safeState.URL != "" {
		safeState.URLHash = urlFingerprint(safeState.URL)
	}
	safeState.URL = ""
	b, err := json.Marshal(&safeState)
	if err != nil {
		return err
	}
	return writeStateAtomically(path, b)
}

// writeStateAtomically keeps a valid resume state on disk if the process exits
// during a checkpoint. commitPart provides the Windows-safe replacement path.
func writeStateAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mnemo-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Keep the existing state-file mode for compatibility while making the
	// content durable before it becomes visible at its final path.
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := commitPart(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func urlFingerprint(rawURL string) string {
	digest := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("%x", digest[:])
}

func stateURLMatches(previous state, rawURL, fingerprint string) bool {
	if previous.URLHash != "" {
		return previous.URLHash == fingerprint
	}
	return previous.URL == rawURL
}

var _ = filepath.Base
