package gdrive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type sessionStore struct {
	state map[string]string
	fail  bool
}

func (s *sessionStore) SaveUploadSession(string, []int) error { return nil }
func (s *sessionStore) LoadUploadSession(string) []int        { return nil }
func (s *sessionStore) ClearUploadSession(key string)         { delete(s.state, key) }
func (s *sessionStore) SaveUploadSessionState(key, value string, _ []int) error {
	if s.fail {
		return errors.New("检查点写入失败")
	}
	s.state[key] = value
	return nil
}
func (s *sessionStore) LoadUploadSessionState(key string) (string, []int) { return s.state[key], nil }

type uploadFixture struct {
	data                             []byte
	received                         int64
	partial                          bool
	loseFinal                        bool
	complete                         bool
	badHash                          bool
	began                            bool
	creates, puts, queries, reserves int
	failBeforeWrite                  bool
	overwrite                        bool
	expire                           bool
}

func (f *uploadFixture) metadata() string {
	hash := netx.MD5Hex(f.data)
	if f.badHash {
		hash = "bad"
	}
	body, _ := json.Marshal(map[string]any{"id": "reserved-id", "name": "file.bin", "mimeType": "application/octet-stream", "parents": []string{"root-id"}, "size": strconv.Itoa(len(f.data)), "md5Checksum": hash})
	return string(body)
}
func (f *uploadFixture) transport(t *testing.T) roundTrip {
	return func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/drive/v3/files/root-id":
			return response(r, 200, `{"id":"root-id","mimeType":"application/vnd.google-apps.folder"}`), nil
		case r.Method == "GET" && r.URL.Path == "/drive/v3/files/generateIds":
			f.reserves++
			return response(r, 200, `{"ids":["reserved-id"]}`), nil
		case r.Method == "GET" && r.URL.Path == "/drive/v3/files":
			if f.overwrite {
				return response(r, 200, `{"files":[{"id":"reserved-id","name":"file.bin","mimeType":"application/octet-stream","size":"1"}]}`), nil
			}
			return response(r, 200, `{"files":[]}`), nil
		case r.Method == "GET" && r.URL.Path == "/drive/v3/files/reserved-id":
			if f.complete {
				return response(r, 200, f.metadata()), nil
			}
			return response(r, 404, `{}`), nil
		case (r.Method == "POST" || r.Method == "PATCH") && strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files"):
			if !f.began {
				t.Fatal("未保存提交记录即写入远端")
			}
			f.creates++
			if f.overwrite && (r.Method != "PATCH" || !strings.HasSuffix(r.URL.Path, "/reserved-id")) {
				t.Fatal("覆盖未使用原文件 ID")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !f.overwrite && body["id"] != "reserved-id" {
				t.Fatal("创建未使用预分配 ID")
			}
			res := response(r, 200, "")
			res.Header.Set("Location", apiHost+"/upload/drive/v3/files?upload_id=opaque-session")
			return res, nil
		case r.Method == "PUT":
			if r.URL.RawQuery != "upload_id=opaque-session" {
				t.Fatal("上传会话地址被改写")
			}
			rangeValue := r.Header.Get("Content-Range")
			if strings.HasPrefix(rangeValue, "bytes */") && len(f.data) > 0 {
				f.queries++
				if f.expire {
					return response(r, 404, `{}`), nil
				}
				if f.complete {
					return response(r, 201, f.metadata()), nil
				}
				res := response(r, 308, "")
				if f.received > 0 {
					res.Header.Set("Range", fmt.Sprintf("bytes=0-%d", f.received-1))
				}
				return res, nil
			}
			f.puts++
			if f.failBeforeWrite {
				return response(r, 403, `{}`), nil
			}
			body, _ := io.ReadAll(r.Body)
			if r.ContentLength != int64(len(body)) {
				t.Fatalf("请求长度错误：%d/%d", r.ContentLength, len(body))
			}
			if len(f.data) == 0 {
				if rangeValue != "bytes */0" {
					t.Fatal("零字节范围无效")
				}
				f.complete = true
				return response(r, 201, f.metadata()), nil
			}
			var start, end, total int64
			if _, err := fmt.Sscanf(rangeValue, "bytes %d-%d/%d", &start, &end, &total); err != nil || start != f.received || total != int64(len(f.data)) || end-start+1 != int64(len(body)) {
				t.Fatalf("分片范围错误：%s；远端偏移 %d", rangeValue, f.received)
			}
			if !bytes.Equal(body, f.data[start:end+1]) {
				t.Fatal("上传内容与源文件不一致")
			}
			if f.partial && f.puts == 1 {
				f.received = 256 * 1024
			} else {
				f.received = end + 1
			}
			if f.received == int64(len(f.data)) {
				f.complete = true
				if f.loseFinal {
					return nil, errors.New("最后回执丢失")
				}
				return response(r, 201, f.metadata()), nil
			}
			res := response(r, 308, "")
			res.Header.Set("Range", fmt.Sprintf("bytes=0-%d", f.received-1))
			return res, nil
		}
		t.Fatalf("意外上传请求 %s %s", r.Method, r.URL.Path)
		return nil, nil
	}
}

func uploadJob(t *testing.T, data []byte) *model.UploadingUI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return &model.UploadingUI{UploadID: "task-id", Info: model.UploadInfo{LocalFilePath: path, Name: "file.bin", ParentFileID: "root", ConflictPolicy: "rename"}}
}

func TestUploadConfirmsRemoteContentAndPartialRanges(t *testing.T) {
	for _, test := range []struct {
		name                                   string
		size                                   int
		partial, loseFinal, badHash, overwrite bool
	}{
		{"零字节", 0, false, false, false, false}, {"普通文件", 3, false, false, false, false}, {"分块部分接收", 6 * 1024 * 1024, true, false, false, false}, {"最终回执丢失", 3, false, true, false, false}, {"校验失败", 3, false, false, true, false}, {"覆盖", 3, false, false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := &uploadFixture{data: bytes.Repeat([]byte("a"), test.size), partial: test.partial, loseFinal: test.loseFinal, badHash: test.badHash, overwrite: test.overwrite}
			useTransport(t, f.transport(t))
			store := &sessionStore{state: map[string]string{}}
			drive.SetUploadSessionStore(store)
			t.Cleanup(func() { drive.SetUploadSessionStore(nil) })
			ui := uploadJob(t, f.data)
			if test.overwrite {
				ui.Info.ConflictPolicy = "overwrite"
			}
			ui.ConfigureUploadAttempt(func() error { f.began = true; return nil })
			err := (&Driver{}).UploadOneFile(t.Context(), account(), ui)
			if (err != nil) != test.badHash {
				t.Fatalf("上传确认结果错误：%v", err)
			}
			if !test.badHash && (ui.Upload.FileID != "reserved-id" || ui.Upload.DownProcess != 100 || len(store.state) != 0) {
				t.Fatalf("完成状态错误：%+v", ui.Upload)
			}
			if test.badHash && len(store.state) == 0 {
				t.Fatal("校验失败清除了检查点")
			}
			if f.creates != 1 {
				t.Fatalf("重复创建了文件：%d", f.creates)
			}
			if test.partial && f.puts != 3 {
				t.Fatalf("未按远端偏移续传：%d", f.puts)
			}
			if test.overwrite && f.reserves != 0 {
				t.Fatal("覆盖生成了新 ID")
			}
		})
	}
}

func TestCheckpointFailureAndAttemptFailurePreventRemoteWrites(t *testing.T) {
	for _, checkpointFailure := range []bool{false, true} {
		t.Run(strconv.FormatBool(checkpointFailure), func(t *testing.T) {
			f := &uploadFixture{data: []byte("abc")}
			useTransport(t, f.transport(t))
			store := &sessionStore{state: map[string]string{}, fail: checkpointFailure}
			drive.SetUploadSessionStore(store)
			t.Cleanup(func() { drive.SetUploadSessionStore(nil) })
			ui := uploadJob(t, f.data)
			ui.ConfigureUploadAttempt(func() error { return errors.New("提交记录保存失败") })
			if err := (&Driver{}).UploadOneFile(t.Context(), account(), ui); err == nil {
				t.Fatal("未阻止写入")
			}
			if f.creates != 0 || f.puts != 0 {
				t.Fatal("本地保存失败后仍写入远端")
			}
		})
	}
}

func TestExpiredSessionReusesReservedFileID(t *testing.T) {
	f := &uploadFixture{data: []byte("abc"), failBeforeWrite: true}
	useTransport(t, f.transport(t))
	store := &sessionStore{state: map[string]string{}}
	drive.SetUploadSessionStore(store)
	t.Cleanup(func() { drive.SetUploadSessionStore(nil) })
	ui := uploadJob(t, f.data)
	ui.ConfigureUploadAttempt(func() error { f.began = true; return nil })
	if err := (&Driver{}).UploadOneFile(t.Context(), account(), ui); err == nil {
		t.Fatal("未返回第一次失败")
	}
	f.failBeforeWrite = false
	f.expire = true
	if err := (&Driver{}).UploadOneFile(t.Context(), account(), ui); err != nil {
		t.Fatal(err)
	}
	if f.reserves != 1 || f.creates != 2 {
		t.Fatalf("未复用同一 ID：%d/%d", f.reserves, f.creates)
	}
}

func TestCompletedUploadCredentialSaveFailureOnlyWarns(t *testing.T) {
	f := &uploadFixture{data: []byte("abc")}
	c := account()
	c.TokenSnapshot = drive.CloneToken(c.Token)
	warnings := 0
	drive.SetTokenUpdater(func(string, string, *model.TokenInfo) error { return errors.New("凭据保存失败") })
	drive.SetUploadWarningHandler(func(_, _ string, issue *model.OperationIssue) {
		if issue.Code != "UPLOAD_CREDENTIAL_SAVE_FAILED" {
			t.Fatal("通知类型错误")
		}
		warnings++
	})
	t.Cleanup(func() { drive.SetTokenUpdater(nil); drive.SetUploadWarningHandler(nil) })
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.oplist.org" {
			return response(r, 200, `{"access_token":"fresh","refresh_token":"rotated"}`), nil
		}
		if r.Header.Get("Authorization") == "Bearer access" {
			return response(r, 401, `{}`), nil
		}
		return response(r, 200, f.metadata()), nil
	})
	cl, _ := newClient(c)
	if err := cl.confirmUpload(t.Context(), uploadSession{FileID: "reserved-id", Name: "file.bin"}, "root-id", netx.MD5Hex(f.data), 3); err != nil {
		t.Fatal(err)
	}
	if warnings != 1 || c.TokenSnapshot.AccessToken != "fresh" {
		t.Fatalf("远端成功后的凭据失败未独立通知：%d", warnings)
	}
}
