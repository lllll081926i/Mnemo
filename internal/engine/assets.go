// Package engine extracts embedded engine assets (mpv binary) to the user
// data dir at first run. aria2c is no longer bundled (native downloader).
package engine

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// BundledMPVVersion must match the MPV_VERSION used by the release workflow.
// The marker prevents re-copying a large app bundle on every player start while
// still replacing an older bundle left by a previous installation.
const BundledMPVVersion = "v0.41.0"

//go:embed all:win32 all:darwin all:linux
var assets embed.FS

// Extract ensures engine files for the current platform exist under dir.
func Extract(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	platform, err := currentPlatform()
	if err != nil {
		return err
	}
	marker := filepath.Join(dir, ".mpv-"+platform+"-version")
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == BundledMPVVersion && bundledReady(dir) {
		return nil
	}
	if err := fs.WalkDir(assets, platform, func(path string, d fs.DirEntry, err error) error {
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
		src, err := assets.Open(path)
		if err != nil {
			return err
		}
		dst, err := os.Create(target)
		if err != nil {
			_ = src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			return err
		}
		if err := dst.Close(); err != nil {
			_ = src.Close()
			return err
		}
		if err := src.Close(); err != nil {
			return err
		}
		if runtime.GOOS != "windows" && filepath.Base(target) == "mpv" {
			return os.Chmod(target, 0o755)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte(BundledMPVVersion+"\n"), 0o644)
}

func currentPlatform() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return "win32", nil
	case "darwin":
		return "darwin", nil
	case "linux":
		return "linux", nil
	default:
		return "", fmt.Errorf("engine: 不支持的平台 %q", runtime.GOOS)
	}
}

func bundledReady(dir string) bool {
	if runtime.GOOS == "darwin" {
		path := filepath.Join(dir, "mpv", "mpv.app", "Contents", "MacOS", "mpv")
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}
	names := []string{"mpv"}
	if runtime.GOOS == "windows" {
		names = []string{"mpv.exe", "mpv.com"}
	}
	ready := false
	for _, name := range names {
		info, err := os.Stat(filepath.Join(dir, "mpv", name))
		if err == nil && !info.IsDir() {
			ready = true
			break
		}
	}
	if !ready {
		return false
	}
	if runtime.GOOS != "linux" {
		return true
	}
	entries, err := os.ReadDir(filepath.Join(dir, "mpv", "lib"))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return true
		}
	}
	return false
}

// MpvPath returns the bundled mpv executable for the current platform.
// Releases must contain this file; a system-wide mpv is deliberately not used
// because that would make codec support and runtime behavior machine-dependent.
func MpvPath(dir string) string {
	if runtime.GOOS == "darwin" {
		// Official macOS archives ship a self-contained mpv.app. Keep the
		// executable inside the bundle so its @rpath dependencies resolve.
		candidate := filepath.Join(dir, "mpv", "mpv.app", "Contents", "MacOS", "mpv")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			_ = os.Chmod(candidate, 0o755)
			return candidate
		}
	}
	names := []string{"mpv"}
	if runtime.GOOS == "windows" {
		names = []string{"mpv.exe"}
	}
	for _, name := range names {
		candidate := filepath.Join(dir, "mpv", name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if runtime.GOOS != "windows" {
				_ = os.Chmod(candidate, 0o755)
			}
			return candidate
		}
	}
	return ""
}

// MpvEnv returns environment overrides required by a bundled Linux player.
func MpvEnv(dir string) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	libDir := filepath.Join(dir, "mpv", "lib")
	if info, err := os.Stat(libDir); err != nil || !info.IsDir() {
		return nil
	}
	return []string{"LD_LIBRARY_PATH=" + libDir + string(os.PathListSeparator) + os.Getenv("LD_LIBRARY_PATH")}
}
