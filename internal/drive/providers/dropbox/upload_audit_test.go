package dropbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestSmallUploadReplaysOpenFileAfterRefresh(t *testing.T) {
	requests, refreshes := 0, 0
	withDropboxTransport(t, dropboxRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/oauth2/token" {
			refreshes++
			return dropboxResponse(request, 200, `{"access_token":"new","expires_in":14400}`), nil
		}
		requests++
		data, err := io.ReadAll(request.Body)
		request.Body.Close()
		if err != nil || string(data) != "file-content" || request.ContentLength != 12 {
			t.Errorf("上传重放正文错误：%q, %d, %v", data, request.ContentLength, err)
		}
		if request.Header.Get("Authorization") == "Bearer old" {
			return dropboxResponse(request, 401, `{"error_summary":"expired_access_token/"}`), nil
		}
		return dropboxResponse(request, 200, `{"id":"file"}`), nil
	}))
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("file-content"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	client, err := clientOf(drive.Context{Token: &model.TokenInfo{AccessToken: "old", RefreshToken: "refresh", DeviceID: builtinAppKey}})
	if err != nil {
		t.Fatal(err)
	}
	fileID, err := client.UploadSmall(context.Background(), "/file", file, 12, uploadPolicy{mode: "overwrite"})
	if err != nil || fileID != "file" || requests != 2 || refreshes != 1 {
		t.Fatalf("文件上传未续办：%q, %v, %d/%d", fileID, err, requests, refreshes)
	}
	if _, err := file.Stat(); err != nil {
		t.Fatal("请求关闭了调用方的上传文件")
	}
}

func TestSingleChunkSessionFinishesWithEmptyBody(t *testing.T) {
	withDropboxTransport(t, dropboxRoundTripper(func(request *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		if request.URL.Path == "/2/files/upload_session/start" {
			if string(data) != "abc" {
				t.Fatal("上传开始正文错误")
			}
			return dropboxResponse(request, 200, `{"session_id":"session"}`), nil
		}
		var argument struct {
			Cursor struct {
				Offset int64 `json:"offset"`
			} `json:"cursor"`
		}
		if err := json.Unmarshal([]byte(request.Header.Get("Dropbox-API-Arg")), &argument); err != nil {
			t.Fatal(err)
		}
		if request.URL.Path != "/2/files/upload_session/finish" || len(data) != 0 || argument.Cursor.Offset != 3 {
			t.Fatalf("已传完分片被重复提交：%s, %q, %+v", request.URL.Path, data, argument)
		}
		return dropboxResponse(request, 200, `{"id":"file"}`), nil
	}))
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	client, err := clientOf(drive.Context{Token: &model.TokenInfo{AccessToken: "token"}})
	if err != nil {
		t.Fatal(err)
	}
	ui := &model.UploadingUI{}
	if err := client.UploadSession(t.Context(), drive.Context{}, file, "/file", 3, ui, uploadPolicy{mode: "overwrite"}); err != nil {
		t.Fatal(err)
	}
	if ui.Upload.FileID != "file" {
		t.Fatal("未保存远端文件 ID")
	}
}
