package aliopen

import (
	"encoding/json"
	"fmt"
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

func TestRapidUploadNegotiatesStructuredPreHashCode(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter })
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/adrive/v1.0/openFile/create" {
			return nil, fmt.Errorf("秒传不应上传分片或再次提交: %s", r.URL.Path)
		}
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		status, response := 200, `{"rapid_upload":true,"file_id":"file","name":"file (1).txt"}`
		if requests == 1 {
			if body["pre_hash"] == nil {
				t.Error("首次协商缺少 pre_hash")
			}
			status, response = 409, `{"code":"PreHashMatched","message":"前缀哈希已匹配，请提交文件证明"}`
		} else if body["proof_code"] == nil || body["pre_hash"] != nil {
			t.Errorf("未正确切换文件证明: %+v", body)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
	})
	account := drive.Context{Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "test", DriveID: "drive"})}}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, ParentFileID: BackupRoot, Name: "file.txt"}}
	if err := (&Driver{}).UploadOneFile(t.Context(), account, ui); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || ui.Upload.FileID != "b:file" || !ui.Info.IsMiaoChuan || ui.Upload.DownProcess != 100 || ui.Info.Name != "file (1).txt" {
		t.Fatalf("秒传回执不完整: requests=%d %+v", requests, ui)
	}
}

func TestRapidUploadMissReusesCreatedSession(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	drive.SetUploadSessionStore(&testUploadSessions{})
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter; drive.SetUploadSessionStore(nil) })
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	hash := netx.SHA1Hex([]byte("payload"))
	creates, puts, commits := 0, 0, 0
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		response := `{}`
		switch r.URL.Path {
		case "/adrive/v1.0/openFile/create":
			creates++
			response = `{"file_id":"file","upload_id":"session"}`
		case "/adrive/v1.0/openFile/getUploadUrl":
			response = `{"part_info_list":[{"part_number":1,"upload_url":"https://upload.example/part"}]}`
		case "/part":
			puts++
			_, _ = io.Copy(io.Discard, r.Body)
		case "/adrive/v1.0/openFile/complete":
			commits++
		default:
			return nil, fmt.Errorf("非预期请求 %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
	})
	account := drive.Context{UserID: "aliopen_test", DriveID: "drive", Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "test", DriveID: "drive"})}}
	driver := &Driver{}
	result, err := driver.RapidUploadByHash(t.Context(), account, drive.RapidUploadRequest{ParentID: BackupRoot, FileName: "file.txt", Method: "sha1", Hash: hash, Size: 7})
	if err != nil || result == nil || result.Reuse {
		t.Fatalf("探测未命中错误: %+v %v", result, err)
	}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, ParentFileID: BackupRoot, Name: "file.txt", ConflictPolicy: "rename"}}
	if err := driver.UploadOneFile(t.Context(), account, ui); err != nil {
		t.Fatal(err)
	}
	if creates != 1 || puts != 1 || commits != 1 || ui.Upload.FileID != "b:file" {
		t.Fatalf("回退重复创建: create=%d put=%d commit=%d %+v", creates, puts, commits, ui.Upload)
	}
}

func TestAliFileParentMappingUsesOneScopePrefix(t *testing.T) {
	for _, tc := range []struct {
		scope        Scope
		parent, want string
	}{
		{ScopeBackup, "b:parent", "b:parent"}, {ScopeResource, "r:parent", "r:parent"},
		{ScopeBackup, BackupRoot, BackupRoot}, {ScopeResource, ResourceRoot, ResourceRoot},
	} {
		files := mapFiles([]aliFile{{FileID: "file", Name: "file.txt"}}, "drive", tc.parent, tc.scope)
		if files[0].ParentFileID != tc.want {
			t.Errorf("目录映射错误: %s -> %s", tc.parent, files[0].ParentFileID)
		}
	}
}

func TestAliIncompleteCreateDoesNotResubmit(t *testing.T) {
	previous, limiter := netx.TestTransportHook, aliOpenLimiter
	aliOpenLimiter = newAliOpenRateLimiter(2, 0)
	t.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter })
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{}`, `{"rapid_upload":true}`, `{"exist":true,"file_id":"other"}`} {
		requests := 0
		netx.TestTransportHook = aliOpenRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			response := body
			if strings.HasSuffix(r.URL.Path, "/get") {
				response = `{"file_id":"other","parent_file_id":"root","type":"file","size":7,"content_hash_name":"sha1","content_hash":"0000000000000000000000000000000000000000"}`
			} else {
				requests++
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
		})
		ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "file.txt", ParentFileID: BackupRoot}}
		err := (&Driver{}).UploadOneFile(t.Context(), drive.Context{Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "token", DriveID: "drive"})}}, ui)
		if err == nil || requests != 1 || ui.Upload.IsCompleted {
			t.Fatalf("不完整回执或不同内容被重复创建/误报成功: body=%s requests=%d err=%v", body, requests, err)
		}
	}
}
