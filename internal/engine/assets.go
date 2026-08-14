// Package engine extracts embedded engine assets (mpv binary) to the user
// data dir at first run. aria2c is no longer bundled (native downloader).
package engine

import (
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed all:win32
var assets embed.FS

// Extract ensures engine files for the current platform exist under dir.
func Extract(dir string) error {
	platform := "win32"
	switch runtime.GOOS {
	case "darwin":
		platform = "darwin"
	case "linux":
		platform = "linux"
	}
	return fs.WalkDir(assets, platform, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(platform, path)
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(target); err == nil {
			return nil // already extracted
		}
		src, err := assets.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return err
		}
		return dst.Close()
	})
}

// MpvPath returns the extracted mpv executable path (win32 only for now).
func MpvPath(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "mpv", "mpv.exe")
	}
	return ""
}
