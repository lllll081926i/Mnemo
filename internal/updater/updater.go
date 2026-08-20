// Package updater checks GitHub releases for a newer version, downloads the
// platform installer/archive with progress, and applies it (silent install +
// restart on Windows; replace binary on mac/linux).
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
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
	repoOwner   = "lllll081926i"
	repoName    = "mnemo-go"
	apiReleases = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
)

// updateSigningPublicKey is injected into release builds with -ldflags. It is
// intentionally empty in local/dev builds; unsigned historical releases keep
// the SHA-256 compatibility path, while a release that ships a signature is
// rejected unless this key is present and valid.
var updateSigningPublicKey string

// Info describes an available update.
type Info struct {
	Version    string `json:"version"`    // e.g. "v0.1.1" (tag name)
	URL        string `json:"url"`        // browser_download_url for this platform
	Size       int64  `json:"size"`       // asset size in bytes
	SHA256     string `json:"sha256"`     // expected checksum (from SHA256SUMS.txt)
	ReleaseURL string `json:"releaseUrl"` // html_url of the release
	Notes      string `json:"notes"`      // release body (unused in UI but available)
}

// assetSuffix returns the download asset filename suffix for the current platform.
func assetSuffix() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return "windows-arm64"
		}
		return "windows-x64"
	case "darwin":
		return "macos-arm64"
	default:
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-x64"
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
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
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
	checksums := ""
	signatureURL := ""
	for _, a := range rel.Assets {
		if a.Name == "SHA256SUMS.txt" {
			checksums = a.BrowserDownloadURL
		}
		if a.Name == "SHA256SUMS.txt.sig" {
			signatureURL = a.BrowserDownloadURL
		}
	}
	shaByName := map[string]string{}
	if checksums != "" {
		values, rawChecksums, checksumErr := fetchChecksums(ctx, checksums)
		if checksumErr != nil {
			return nil, fmt.Errorf("fetch release checksums: %w", checksumErr)
		}
		if signatureURL == "" && releaseSignatureRequired() {
			return nil, fmt.Errorf("release signature is missing")
		}
		if signatureURL != "" {
			signature, signatureErr := fetchBody(ctx, signatureURL, 128<<10)
			if signatureErr != nil {
				return nil, fmt.Errorf("fetch release signature: %w", signatureErr)
			}
			if signatureErr = verifyReleaseSignature(rawChecksums, signature); signatureErr != nil {
				return nil, fmt.Errorf("release signature verification failed: %w", signatureErr)
			}
		}
		shaByName = values
	}
	makeInfo := func(name, downloadURL string, size int64) *Info {
		if shaByName[name] == "" {
			return nil
		}
		return &Info{Version: rel.TagName, URL: downloadURL, Size: size, SHA256: shaByName[name], ReleaseURL: rel.HTMLURL, Notes: rel.Body}
	}
	// Windows releases intentionally provide only the native installer.
	if runtime.GOOS == "windows" {
		for _, a := range rel.Assets {
			if strings.Contains(a.Name, suffix) && strings.HasSuffix(a.Name, "-Setup.exe") {
				if info := makeInfo(a.Name, a.BrowserDownloadURL, a.Size); info != nil {
					return info, nil
				}
			}
		}
		return nil, nil
	}
	for _, a := range rel.Assets {
		if strings.Contains(a.Name, suffix) && (strings.HasSuffix(a.Name, ".tar.gz") || strings.HasSuffix(a.Name, ".deb")) {
			if info := makeInfo(a.Name, a.BrowserDownloadURL, a.Size); info != nil {
				return info, nil
			}
		}
	}
	return nil, nil
}

func fetchChecksums(ctx context.Context, rawURL string) (map[string]string, []byte, error) {
	b, err := fetchBody(ctx, rawURL, 2<<20)
	if err != nil {
		return nil, nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && len(fields[0]) == 64 {
			result[fields[len(fields)-1]] = strings.ToLower(fields[0])
		}
	}
	return result, b, nil
}

func fetchBody(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func releaseSignatureRequired() bool {
	return strings.TrimSpace(updateSigningPublicKey) != ""
}

func verifyReleaseSignature(message, encodedSignature []byte) error {
	keyText := strings.TrimSpace(updateSigningPublicKey)
	if keyText == "" {
		return fmt.Errorf("应用未配置更新签名公钥")
	}
	publicKey, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("更新签名公钥格式无效")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedSignature)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("更新签名格式无效")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), message, signature) {
		return fmt.Errorf("签名内容不匹配")
	}
	return nil
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

// IsDownloadPath accepts only files created beneath the app's update cache.
func IsDownloadPath(dataDir, path string) bool {
	root, rootErr := filepath.Abs(DownloadDir(dataDir))
	target, targetErr := filepath.Abs(path)
	if rootErr != nil || targetErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// DownloadDir returns a writable temp directory for the update download.
func DownloadDir(dataDir string) string {
	d := filepath.Join(dataDir, "updates")
	_ = os.MkdirAll(d, 0o755)
	return d
}
