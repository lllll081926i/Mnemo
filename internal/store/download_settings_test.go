package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyDownloadSettingsKeepIndependentConnectionDefault(t *testing.T) {
	root := t.TempDir()
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, settingsFile), []byte(`{"maxConcurrentDownloads":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := st.GetSettings()
	if err != nil || settings.MaxConcurrentDownloads != 1 || settings.MaxDownloadConnections != 8 {
		t.Fatalf("旧设置不兼容: %+v，%v", settings, err)
	}
	settings.MaxDownloadConnections = 12
	if err := st.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	reloaded, err := st.GetSettings()
	if err != nil || reloaded.MaxDownloadConnections != 12 {
		t.Fatalf("连接设置未持久化: %+v，%v", reloaded, err)
	}
}
