package dropbox

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestSkipUsesServerSideRefusalWhenTargetAppears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	writes := 0
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "get_metadata") {
			return dropboxResponse(r, 409, `{"error_summary":"path/not_found/"}`), nil
		}
		writes++
		var arg map[string]any
		if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg); err != nil {
			t.Fatal(err)
		}
		if arg["mode"] != "add" || arg["strict_conflict"] != true || arg["autorename"] != false {
			t.Fatalf("跳过仍允许覆盖: %+v", arg)
		}
		return dropboxResponse(r, 409, `{"error_summary":"path/conflict/file/"}`), nil
	}))
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "file", ConflictPolicy: "skip"}}
	if err := (&Driver{}).UploadOneFile(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "access"}}, ui); err != nil || writes != 1 {
		t.Fatalf("并发名称冲突未按策略跳过: %v %d", err, writes)
	}
}

type failingUploadStore struct{}

func (*failingUploadStore) SaveUploadSession(string, []int) error {
	return errors.New("检查点保存失败")
}
func (*failingUploadStore) LoadUploadSession(string) []int { return nil }
func (*failingUploadStore) ClearUploadSession(string)      {}
func (*failingUploadStore) SaveUploadSessionState(string, string, []int) error {
	return errors.New("检查点保存失败")
}
func (*failingUploadStore) LoadUploadSessionState(string) (string, []int) { return "", nil }

func TestSessionCheckpointFailureStopsBeforeCommit(t *testing.T) {
	drive.SetUploadSessionStore(&failingUploadStore{})
	t.Cleanup(func() { drive.SetUploadSessionStore(nil) })
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/start") {
			t.Fatal("检查点保存失败后仍追加或提交")
		}
		return dropboxResponse(r, 200, `{"session_id":"session"}`), nil
	}))
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := newClient("access").UploadSession(t.Context(), drive.Context{}, f, "/file", 4, &model.UploadingUI{}, uploadPolicy{mode: "add"}); err == nil || !strings.Contains(err.Error(), "保存") {
		t.Fatalf("忽略检查点错误: %v", err)
	}
}
