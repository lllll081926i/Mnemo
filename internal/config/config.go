// Package config loads runtime configuration: secrets (OAuth client ids) and
// app directories.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AppVersion is the application version. Should match wails.json and git tags.
const AppVersion = "0.1.0"

// Secrets holds OAuth application credentials, loaded from secrets.json in the
// data dir. Keys match the legacy app.
type Secrets struct {
	OnedriveClientID string `json:"onedrive_client_id"`
	DropboxAppKey    string `json:"dropbox_app_key"`
}

// LoadSecrets reads secrets.json from dir if present.
func LoadSecrets(dir string) Secrets {
	var s Secrets
	b, err := os.ReadFile(filepath.Join(dir, "secrets.json"))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}

// UserConfigDir returns the per-user system config directory (e.g.
// %AppData%/Mnemo on Windows, ~/Library/Application Support/Mnemo on macOS).
// Login credentials (accounts.json) live here so they persist across installs
// and are independent of the install location.
func UserConfigDir(appName string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// DataDir returns the application data directory, co-located with the
// executable (installDir/data). All non-credential data — settings, tags,
// favorites, tasks, shares, sync configs, engine binaries, logs — lives here
// so it is kept with the installation and moves with it.
//
// fallbackDir is used when the executable directory is not writable (e.g.
// during development); it falls back to the user config dir.
func DataDir(appName, fallbackDir string) (string, error) {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "data")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir, nil
		}
	}
	// fallback: user config dir / data
	dir := filepath.Join(fallbackDir, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// UserDataDir is a legacy alias kept for compatibility; returns the user
// config dir. Prefer UserConfigDir + DataDir.
func UserDataDir(appName string) (string, error) {
	return UserConfigDir(appName)
}
