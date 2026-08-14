// Package config loads runtime configuration: secrets (OAuth client ids) and
// app directories.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Secrets holds OAuth application credentials, loaded from secrets.json next
// to the executable or in the user config dir. Keys match the legacy app.
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

// UserDataDir returns the per-user data directory.
func UserDataDir(appName string) (string, error) {
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
