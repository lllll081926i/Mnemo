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
