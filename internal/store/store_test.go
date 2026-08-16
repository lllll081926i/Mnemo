package store

import (
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
