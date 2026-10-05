package aliopen

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestUploadResumeCountsCompletedPartsOnce(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter; drive.SetUploadSessionStore(nil) })
	path := filepath.Join(t.TempDir(), "file")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	const size = 20*1024*1024 + 1
	err = file.Truncate(size)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := netx.HashFile(path, netx.HashSHA1)
	if err != nil {
		t.Fatal(err)
	}
	account := drive.Context{UserID: "aliopen:user", DriveID: "drive", Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "token", DriveID: "drive"})}}
	sessionName := "file.txt\x00ignore\x00" + strings.ToUpper(hash)
	sessions := &testUploadSessions{key: drive.UploadSessionKey(account.UserID, account.DriveID, "b:root", sessionName, size), id: encodeAliOpenUploadSession("session", "file"), parts: []int{1}}
	drive.SetUploadSessionStore(sessions)
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, ParentFileID: "b:root", Name: "file.txt"}}
	puts := 0
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		switch {
		case request.Method == http.MethodPut:
			puts++
			if ui.Upload.DownSize != size-1 {
				t.Errorf("续传重复计入已完成分片：done=%d", ui.Upload.DownSize)
			}
			if _, err := io.Copy(io.Discard, request.Body); err != nil {
				return nil, err
			}
		case request.URL.Path == "/adrive/v1.0/openFile/getUploadUrl":
			body = `{"part_info_list":[{"part_number":2,"upload_url":"https://upload.example/part"}]}`
		case request.URL.Path == "/adrive/v1.0/openFile/complete":
		default:
			t.Fatalf("续传不应新建文件：%s", request.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	if err := (&Driver{}).UploadOneFile(t.Context(), account, ui); err != nil {
		t.Fatal(err)
	}
	if puts != 1 || ui.Upload.DownSize != size {
		t.Fatalf("续传错误：puts=%d, done=%d", puts, ui.Upload.DownSize)
	}
}

func TestUploadDoesNotTrustCachedHashAfterSourceChange(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter })
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if request.URL.Path != "/adrive/v1.0/openFile/create" || body["content_hash"] != strings.ToUpper(netx.SHA1Hex([]byte("new"))) {
			t.Fatalf("使用了旧文件 SHA1：%+v", body)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"rapid_upload":true,"file_id":"file"}`)), Request: request}, nil
	})
	account := drive.Context{Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "token", DriveID: "drive"})}}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, ParentFileID: "b:root", Name: "file.txt", SHA1: netx.SHA1Hex([]byte("old"))}}
	if err := (&Driver{}).UploadOneFile(t.Context(), account, ui); err != nil {
		t.Fatal(err)
	}
}
