package aliopen

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestShareSaveUsesSelectedBackupOrResourceDrive(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter })
	for _, destination := range []struct{ parent, driveID string }{{"b:folder", "backup-drive"}, {"r:folder", "resource-drive"}} {
		t.Run(destination.parent, func(t *testing.T) {
			netx.TestTransportHook = aliOpenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if request.URL.Path != "/adrive/v1.0/openFile/copy" || body["to_drive_id"] != destination.driveID || body["to_parent_file_id"] != "folder" {
					t.Fatalf("转存写入了错误的云盘空间: %+v", body)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"file_id":"saved"}`)), Request: request}, nil
			})
			account := drive.Context{Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "token", DriveID: "default", BackupDriveID: "backup-drive", ResourceDriveID: "resource-drive"})}}
			session := &drive.ShareImportSession{Provider: providerID, ShareID: "share", ShareToken: "share-token"}
			if _, err := (&Driver{}).SaveShare(t.Context(), account, session, []string{"file"}, destination.parent); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShareImportUsesPasswordFromURL(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter })
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		response := `{"items":[]}`
		if request.URL.Path == "/adrive/v1.0/openFile/getShareToken" {
			if body["share_pwd"] != "abcd" {
				t.Fatalf("丢失链接中的提取码: %+v", body)
			}
			response = `{"share_token":"token"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
	})
	account := drive.Context{Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "token", DriveID: "drive"})}}
	session, err := (&Driver{}).ImportShareSession(t.Context(), account, "https://www.alipan.com/s/share?pwd=abcd", "")
	if err != nil || session.Password != "abcd" {
		t.Fatalf("链接提取码未生效: %v", err)
	}
}
