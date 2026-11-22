package app

import (
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestExpiredCredentialsPreserveAccountAndIdentity(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := &model.Account{
		UserID: "pikpak_account", DriveID: "pikpak_account",
		CustomName: "我的账号", CustomIcon: "icon.svg", Order: 7,
		Token: &model.TokenInfo{TokenFrom: "pikpak", AccessToken: "expired"},
	}
	if err := st.SaveAccount(original); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.store = st
	a.handleDriveOperationError(original.UserID, original.DriveID, drive.ErrUnauthorized)
	current, err := st.GetAccount(original.UserID)
	if err != nil || current == nil {
		t.Fatalf("expired account should remain in store: %v", err)
	}
	if current.AuthState != "reauth_required" || current.CustomName != original.CustomName ||
		current.CustomIcon != original.CustomIcon || current.Order != original.Order ||
		current.DriveID != original.DriveID {
		t.Fatalf("expired account identity changed: %+v", current)
	}
	if current.Token == nil || current.Token.AccessToken != "expired" {
		t.Fatalf("existing credentials lost before reauthorization: %+v", current.Token)
	}
	a.handleDriveOperationError(original.UserID, original.DriveID, drive.ErrUnauthorized)
	after, err := st.GetAccount(original.UserID)
	if err != nil || after == nil || after.AuthState != "reauth_required" {
		t.Fatalf("repeated expiration destroyed account: %+v %v", after, err)
	}
}

func TestReauthorizationUpdatesOnlyExistingAccountAndSpace(t *testing.T) {
	state, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := &model.Account{UserID: "pikpak_original", DriveID: "original-drive", CustomName: "原账号", CustomIcon: "icon.svg", Order: 7, AuthState: "reauth_required", Token: &model.TokenInfo{TokenFrom: "pikpak", AccessToken: "old"}}
	if err := state.SaveAccount(original); err != nil {
		t.Fatal(err)
	}
	token := &model.TokenInfo{TokenFrom: "pikpak", AccessToken: "new"}
	if _, err := state.ReauthorizeAccount(original.UserID, "wrong-drive", token); err == nil {
		t.Fatal("接受了不同网盘空间")
	}
	if _, err := state.ReauthorizeAccount("pikpak_missing", original.DriveID, token); err == nil {
		t.Fatal("重新授权创建了新账号")
	}
	unchanged, err := state.GetAccount(original.UserID)
	if err != nil || unchanged.AuthState != "reauth_required" || unchanged.Token.AccessToken != "old" {
		t.Fatalf("失败的授权修改了原账号: %v", err)
	}
	updated, err := state.ReauthorizeAccount(original.UserID, original.DriveID, token)
	if err != nil || updated.AuthState != "" || updated.Token.AccessToken != "new" || updated.CustomName != original.CustomName || updated.CustomIcon != original.CustomIcon || updated.Order != original.Order || updated.DriveID != original.DriveID {
		t.Fatalf("重新授权未保留原账号: %v", err)
	}
}
