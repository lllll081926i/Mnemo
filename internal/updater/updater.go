// Package updater checks GitHub releases for a newer version, downloads the
// platform installer/archive with progress, and applies it (silent install +
// restart on Windows; replace binary on mac/linux).
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"mnemo-go/internal/config"
)

const (
	repoOwner  = "lllll081926i"
	repoName   = "mnemo-go"
	apiReleases = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
)

// Info describes an available update.
type Info struct {
	Version    string `json:"version"`     // e.g. "v0.1.1" (tag name)
	URL        string `json:"url"`         // browser_download_url for this platform
	Size       int64  `json:"size"`        // asset size in bytes
	SHA256     string `json:"sha256"`      // expected checksum (from SHA256SUMS.txt)
	ReleaseURL string `json:"releaseUrl"`  // html_url of the release
	Notes      string `json:"notes"`       // release body (unused in UI but available)
}

// assetSuffix returns the download asset filename suffix for the current platform.
func assetSuffix() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	default:
		return "linux"
	}
}
func currentVersion() string {
	return "v" + config.AppVersion
}

// Check queries GitHub for the latest release and returns an Info if a newer
// version is available. Returns nil Info (no error) when already up-to-date.
func Check(ctx context.Context) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", apiReleases, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}
	var rel struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" || rel.TagName == currentVersion() {
		return nil, nil
	}
	suffix := assetSuffix()
	// prefer the Inno Setup installer on Windows
	if runtime.GOOS == "windows" {
		for _, a := range rel.Assets {
			if strings.Contains(a.Name, suffix) && strings.HasSuffix(a.Name, "-Setup.exe") {
				return &Info{Version: rel.TagName, URL: a.BrowserDownloadURL, Size: a.Size, ReleaseURL: rel.HTMLURL, Notes: rel.Body}, nil
			}
		}
	}
	for _, a := range rel.Assets {
		if strings.Contains(a.Name, suffix) {
			return &Info{Version: rel.TagName, URL: a.BrowserDownloadURL, Size: a.Size, ReleaseURL: rel.HTMLURL, Notes: rel.Body}, nil
		}
	}
	return nil, nil
}

// Progress is emitted during download.
type Progress struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
}

// Download fetches the update to dest and calls onProgress with progress.
// Returns the path to the downloaded file.
func Download(ctx context.Context, url, dest string, onProgress func(Progress)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	total := resp.ContentLength
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		select {
		case <-ctx.Done():
			out.Close()
			os.Remove(dest)
			return "", ctx.Err()
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				os.Remove(dest)
				return "", werr
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(Progress{Downloaded: downloaded, Total: total})
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			os.Remove(dest)
			return "", err
		}
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	// verify checksum if sha256 file is available (optional, best-effort)
	return dest, nil
}

// VerifyChecksum computes the SHA-256 of file and compares to expected.
func VerifyChecksum(path, expected string) (bool, error) {
	if expected == "" {
		return true, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	return strings.EqualFold(got, expected), nil
}

// Apply launches the downloaded installer and exits the app.
// Windows: run the Inno Setup exe with /SILENT /CLOSEAPPLICATIONS /RESTARTAPPLICATIONS.
// macOS/Linux: not supported via installer — the caller should prompt the user
// to manually replace (or the archive is extracted in place).
func Apply(installerPath string) error {
	switch runtime.GOOS {
	case "windows":
		return applyWindows(installerPath)
	default:
		return applyUnix(installerPath)
	}
}

// DownloadDir returns a writable temp directory for the update download.
func DownloadDir(dataDir string) string {
	d := filepath.Join(dataDir, "updates")
	_ = os.MkdirAll(d, 0o755)
	return d
}
