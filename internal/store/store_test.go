package store

import (
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/model"
)

func TestOpenAndSettings(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// default settings
	s, err := st.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.MaxConcurrentDownloads <= 0 {
		t.Error("default MaxConcurrentDownloads should be positive")
	}
	// update settings
	s.Proxy = "http://127.0.0.1:7890"
	s.MaxDownloadSpeed = 1024000
	if err := st.SetSettings(s); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	// verify persistence
	s2, _ := st.GetSettings()
	if s2.Proxy != "http://127.0.0.1:7890" {
		t.Errorf("Proxy not persisted: %s", s2.Proxy)
	}
	if s2.MaxDownloadSpeed != 1024000 {
		t.Errorf("MaxDownloadSpeed not persisted: %d", s2.MaxDownloadSpeed)
	}
}

func TestUploadSessionStore(t *testing.T) {
	dir := t.TempDir()
	InitUploadSessions(dir)
	key := "testkey123"
	parts := []int{1, 3, 5}
	if err := SaveUploadSession(key, parts); err != nil {
		t.Fatalf("SaveUploadSession: %v", err)
	}
	got := LoadUploadSession(key)
	if len(got) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(got))
	}
	if got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Errorf("parts mismatch: %v", got)
	}
	ClearUploadSession(key)
	got2 := LoadUploadSession(key)
	if len(got2) > 0 {
		t.Error("session should be cleared")
	}
}

func TestAccountSaveLoad(t *testing.T) {
	dir := t.TempDir()
	st, _ := Open(dir)
	acc := &model.Account{
		UserID: "pikpak_test123",
		Token: &model.TokenInfo{
			UserName:          "testuser",
			ProviderAccountID: "123",
		},
	}
	if err := st.SaveAccount(acc); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	list, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 account, got %d", len(list))
	}
	if list[0].UserID != "pikpak_test123" {
		t.Errorf("UserID mismatch: %s", list[0].UserID)
	}
	got, err := st.GetAccount("pikpak_test123")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.Token.UserName != "testuser" {
		t.Errorf("UserName mismatch: %s", got.Token.UserName)
	}
	_ = filepath.Join(dir, "unused")
}

func TestListAccountsIgnoresNullEntries(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Plaintext is a supported legacy format and is intentionally used here
	// to exercise recovery of a partially corrupted account list.
	content := `[
  null,
  {"user_id":"valid-account","order":1}
]`
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write legacy accounts: %v", err)
	}
	list, err := st.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(list) != 1 || list[0] == nil || list[0].UserID != "valid-account" {
		t.Fatalf("unexpected accounts: %#v", list)
	}
	account, err := st.GetAccount("valid-account")
	if err != nil || account == nil {
		t.Fatalf("GetAccount: account=%#v err=%v", account, err)
	}
}

func TestDirectoryCacheIsolationAndClear(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	files := []model.File{{FileID: "file-1", Name: "one.txt"}}
	if err := st.SaveDirectoryCache("provider|account-a|drive-a|list|root|", files); err != nil {
		t.Fatalf("SaveDirectoryCache: %v", err)
	}
	got, err := st.LoadDirectoryCache("provider|account-a|drive-a|list|root|")
	if err != nil {
		t.Fatalf("LoadDirectoryCache: %v", err)
	}
	if len(got) != 1 || got[0].FileID != "file-1" {
		t.Fatalf("cached files mismatch: %#v", got)
	}
	other, err := st.LoadDirectoryCache("provider|account-b|drive-a|list|root|")
	if err != nil {
		t.Fatalf("LoadDirectoryCache(other): %v", err)
	}
	if other != nil {
		t.Fatalf("cache leaked between accounts: %#v", other)
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "directories")); err != nil {
		t.Fatalf("cache directory missing: %v", err)
	}
	if err := st.DeleteDirectoryCache("provider|account-a|drive-a|list|root|"); err != nil {
		t.Fatalf("DeleteDirectoryCache: %v", err)
	}
	deleted, err := st.LoadDirectoryCache("provider|account-a|drive-a|list|root|")
	if err != nil {
		t.Fatalf("LoadDirectoryCache after delete: %v", err)
	}
	if deleted != nil {
		t.Fatalf("cache still present after delete: %#v", deleted)
	}
	if err := st.SaveDirectoryCache("provider|account-a|drive-a|list|root|", files); err != nil {
		t.Fatalf("SaveDirectoryCache after delete: %v", err)
	}
	if err := st.SetSettings(Settings{}); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	if err := st.ClearCache(); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	cleared, err := st.LoadDirectoryCache("provider|account-a|drive-a|list|root|")
	if err != nil {
		t.Fatalf("LoadDirectoryCache after clear: %v", err)
	}
	if cleared != nil {
		t.Fatalf("cache still present after clear: %#v", cleared)
	}
	if _, err := st.GetSettings(); err != nil {
		t.Fatalf("ClearCache removed settings: %v", err)
	}
}
