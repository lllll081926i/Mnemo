package thunder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

func uploadSource(t *testing.T, size int64) *model.UploadingUI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "测试.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "测试.txt", ParentFileID: RootID, ConflictPolicy: "rename", Size: size}}
}

func TestThunderLocalUploadReceipts(t *testing.T) {
	for _, mode := range []string{"rapid", "verified", "put", "multipart", "missing_id", "unconfirmed", "incomplete_credentials", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			size := int64(7)
			if mode == "multipart" {
				size = 16<<20 + 1
			}
			ui := uploadSource(t, size)
			hash, err := driveutil.ComputeGCID(context.Background(), ui.Info.LocalFilePath, nil)
			if err != nil {
				t.Fatal(err)
			}
			writes, puts, parts, commits, details, attempts := 0, 0, 0, 0, 0, 0
			ui.ConfigureUploadAttempt(func() error { attempts++; return nil })
			mockHTTP(t, func(r *http.Request) (*http.Response, error) {
				body := `{}`
				if strings.HasSuffix(r.URL.Host, "storage.test") {
					if r.Header.Get("Authorization") == "" || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.Header.Get("X-Amz-Security-Token") != "sts" {
						t.Fatalf("对象存储鉴权错误: %+v", r.Header)
					}
					var n int64
					if r.Body != nil {
						n, _ = io.Copy(io.Discard, r.Body)
					}
					q := r.URL.Query()
					switch {
					case r.Method == http.MethodPut && q.Get("partNumber") != "":
						parts++
						if n < 1 {
							t.Fatal("分片数据为空")
						}
						body = ""
					case r.Method == http.MethodPost && q.Has("uploads"):
						body = `<InitiateMultipartUploadResult><UploadId>upload-session</UploadId></InitiateMultipartUploadResult>`
					case r.Method == http.MethodPost && q.Get("uploadId") != "":
						commits++
						body = `<CompleteMultipartUploadResult><ETag>done</ETag></CompleteMultipartUploadResult>`
					case r.Method == http.MethodPut:
						puts++
						if n != size {
							t.Fatalf("上传字节数=%d 预期=%d", n, size)
						}
					default:
						t.Fatalf("非预期对象请求: %s %s", r.Method, r.URL)
					}
					result := response(r, 200, body)
					result.Header.Set("ETag", `"part"`)
					return result, nil
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/drive/v1/files":
					body = `{"files":[]}`
				case r.Method == http.MethodPost && r.URL.Path == "/drive/v1/files":
					writes++
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Fatal(err)
					}
					if request["hash"] != hash || request["parent_id"] != "" || request["upload_type"] != "UPLOAD_TYPE_RESUMABLE" {
						t.Fatalf("上传请求错误: %+v", request)
					}
					switch mode {
					case "rapid":
						body = `{"file":{"id":"file","phase":"PHASE_TYPE_COMPLETE"}}`
					case "missing_id":
						body = `{"file":{"phase":"PHASE_TYPE_COMPLETE"}}`
					case "rejected":
						return response(r, 403, `{"error_code":100,"error":"vip_required","error_description":"需要会员"}`), nil
					case "put", "multipart", "incomplete_credentials":
						body = `{"upload_type":"UPLOAD_TYPE_RESUMABLE","file":{"id":"file","phase":"PHASE_TYPE_RUNNING"},"resumable":{"params":{"access_key_id":"ak","access_key_secret":"secret","security_token":"sts","bucket":"bucket","endpoint":"https://bucket.storage.test","key":"object"}}}`
						if mode == "incomplete_credentials" {
							body = strings.Replace(body, `"access_key_id":"ak"`, `"access_key_id":""`, 1)
						}
					default:
						body = `{"file":{"id":"file"}}`
					}
				case r.URL.Path == "/drive/v1/files/file":
					details++
					phase := "PHASE_TYPE_COMPLETE"
					if mode == "unconfirmed" {
						phase = "PHASE_TYPE_ERROR"
					}
					body = fmt.Sprintf(`{"id":"file","phase":%q,"size":%d,"hash":%q}`, phase, size, hash)
				default:
					t.Fatalf("非预期请求: %s %s", r.Method, r.URL)
				}
				return response(r, 200, body), nil
			})
			err = (&Driver{}).UploadOneFile(context.Background(), accountFor("upload"), ui)
			wantFailure := mode == "missing_id" || mode == "unconfirmed" || mode == "incomplete_credentials" || mode == "rejected"
			if (err != nil) != wantFailure || writes != 1 || attempts != 1 || ui.Upload.IsCompleted == wantFailure {
				t.Fatalf("上传回执错误: err=%v writes=%d attempts=%d %+v", err, writes, attempts, ui)
			}
			if mode == "rapid" && (puts != 0 || details != 0 || !ui.Info.IsMiaoChuan) {
				t.Fatal("秒传命中后继续请求")
			}
			if mode == "verified" && (details != 1 || !ui.Info.IsMiaoChuan) {
				t.Fatal("空参数回执未经复核")
			}
			if mode == "put" && (puts != 1 || details != 1 || ui.Info.IsMiaoChuan) {
				t.Fatal("普通上传未等待确认")
			}
			if mode == "multipart" && (parts != 2 || commits != 1 || details != 1) {
				t.Fatalf("分片没有完整提交: %d %d %d", parts, commits, details)
			}
		})
	}
}

func TestThunderUploadCredentialFailureKeepsCompletedReceipt(t *testing.T) {
	ui := uploadSource(t, 1)
	creates, warnings := 0, 0
	drive.SetTokenUpdater(func(string, string, *model.TokenInfo) error { return errors.New("存储故障") })
	drive.SetUploadWarningHandler(func(string, string, *model.OperationIssue) { warnings++ })
	t.Cleanup(func() { drive.SetTokenUpdater(nil); drive.SetUploadWarningHandler(nil) })
	mockHTTP(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/shield/captcha/init") {
			return response(r, 200, `{"captcha_token":"new"}`), nil
		}
		if r.Method == http.MethodGet {
			return response(r, 200, `{"files":[]}`), nil
		}
		creates++
		if creates == 1 {
			return response(r, 400, `{"error_code":9,"error":"captcha_required"}`), nil
		}
		return response(r, 200, `{"file":{"id":"file","phase":"PHASE_TYPE_COMPLETE"}}`), nil
	})
	if err := (&Driver{}).UploadOneFile(context.Background(), accountFor("save"), ui); err != nil || !ui.Upload.IsCompleted || creates != 2 || warnings != 1 {
		t.Fatalf("秒传成功被凭据错误覆盖: %v creates=%d warnings=%d %+v", err, creates, warnings, ui)
	}
}

func TestThunderUploadConflictBeforeAttempt(t *testing.T) {
	for _, policy := range []string{"skip", "refuse", "overwrite"} {
		t.Run(policy, func(t *testing.T) {
			ui := uploadSource(t, 1)
			ui.Info.ConflictPolicy = policy
			ui.ConfigureUploadAttempt(func() error { t.Fatal("冲突检查前不得记录写入"); return nil })
			mockHTTP(t, func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet {
					t.Fatal("冲突时不得写入")
				}
				return response(r, 200, `{"files":[{"id":"existing","name":"测试.txt"}]}`), nil
			})
			err := (&Driver{}).UploadOneFile(context.Background(), accountFor("conflict"), ui)
			if (err == nil) != (policy == "skip") {
				t.Fatalf("冲突策略错误: %s %v", policy, err)
			}
		})
	}
}
