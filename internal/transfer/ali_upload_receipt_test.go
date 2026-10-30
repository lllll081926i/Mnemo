package transfer

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers/aliopen"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type aliReceiptTransport func(*http.Request) (*http.Response, error)

func (f aliReceiptTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAliRapidSuccessSurvivesCredentialSaveFailure(t *testing.T) {
	q, source, _ := newReceiptQueue(t)
	previous := netx.TestTransportHook
	t.Cleanup(func() {
		netx.TestTransportHook = previous
		drive.SetTokenResolver(nil)
		drive.SetTokenUpdater(nil)
		drive.SetUploadWarningHandler(nil)
	})
	drive.SetTokenResolver(func(string, string) (*model.TokenInfo, error) {
		return &model.TokenInfo{TokenFrom: model.ProviderAliopen, RefreshToken: `{"access_token":"test","drive_id":"drive"}`}, nil
	})
	drive.SetTokenUpdater(func(string, string, *model.TokenInfo) error { return errors.New("模拟凭据存储故障") })
	warnings := 0
	drive.SetUploadWarningHandler(func(user, disk string, issue *model.OperationIssue) {
		warnings++
		if user != "aliopen_test" || disk != "drive" || issue.Code != "UPLOAD_CREDENTIAL_SAVE_FAILED" {
			t.Errorf("警告作用域错误: %s %s %+v", user, disk, issue)
		}
	})
	creates := 0
	netx.TestTransportHook = aliReceiptTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/adrive/v1.0/openFile/create" {
			return nil, errors.New("秒传成功后不应再次写入")
		}
		creates++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"rapid_upload":true,"file_id":"file"}`)), Request: r}, nil
	})
	id := q.enqueue("aliopen_test", "drive", "b:parent", "rename", source, "源文件.txt", 14).UploadID
	q.workers.Wait()
	job := q.List()[0]
	if !job.Upload.IsCompleted || job.Upload.IsFailed || job.Upload.Attempt.State != "confirmed" || job.Upload.Issue != nil || !job.Info.IsMiaoChuan || creates != 1 || warnings != 1 {
		t.Fatalf("远端秒传成功被误报: creates=%d warnings=%d %+v", creates, warnings, job)
	}
	if err := q.Resume(id); err == nil {
		t.Fatal("已完成的上传不得重新启动")
	}
	q.workers.Wait()
	if creates != 1 {
		t.Fatal("已完成的秒传被重复提交")
	}
	saved, err := q.store.ListUploadTasks()
	if err != nil || len(saved) != 1 || saved[0].Upload.Attempt.State != "confirmed" {
		t.Fatalf("完成回执未持久化: %+v %v", saved, err)
	}
}

func TestAliPreflightFailureDoesNotRequireTargetReview(t *testing.T) {
	for _, parent := range []string{"b:parent", "aliopen_root"} {
		t.Run(parent, func(t *testing.T) {
			q, source, _ := newReceiptQueue(t)
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous; drive.SetTokenResolver(nil) })
			requests := 0
			netx.TestTransportHook = aliReceiptTransport(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("不应发送远端请求")
			})
			drive.SetTokenResolver(func(string, string) (*model.TokenInfo, error) {
				return &model.TokenInfo{TokenFrom: model.ProviderAliopen}, nil
			})
			q.enqueue("aliopen_test", "drive", parent, "rename", source, "源文件.txt", 14)
			q.workers.Wait()
			job := q.List()[0]
			if requests != 0 || !job.Upload.IsFailed || job.Upload.Attempt.State != "" || job.Upload.Issue != nil && job.Upload.Issue.Code == "UPLOAD_RESULT_UNKNOWN" {
				t.Fatalf("发送前失败不应显示待核对: requests=%d %+v", requests, job)
			}
		})
	}
}

func TestAliLostReceiptReconcilesExactHashWithoutResubmit(t *testing.T) {
	q, source, _ := newReceiptQueue(t)
	hash, err := netx.HashFile(source, netx.HashSHA1)
	if err != nil {
		t.Fatal(err)
	}
	previous := netx.TestTransportHook
	drive.ClearFileMetaCache()
	t.Cleanup(func() { netx.TestTransportHook = previous; drive.SetTokenResolver(nil); drive.ClearFileMetaCache() })
	drive.SetTokenResolver(func(string, string) (*model.TokenInfo, error) {
		return &model.TokenInfo{TokenFrom: model.ProviderAliopen, RefreshToken: `{"access_token":"test","drive_id":"drive","backup_drive_id":"drive"}`}, nil
	})
	creates := 0
	netx.TestTransportHook = aliReceiptTransport(func(r *http.Request) (*http.Response, error) {
		body := `{}`
		switch {
		case r.Method == http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
		case r.URL.Path == "/adrive/v1.0/openFile/create":
			creates++
			body = `{"file_id":"file","upload_id":"session","part_info_list":[{"part_number":1,"upload_url":"https://upload.example/part"}]}`
		case r.URL.Path == "/adrive/v1.0/openFile/complete":
			return nil, errors.New("模拟完成回执丢失")
		case r.URL.Path == "/adrive/v1.0/openFile/get":
			body = `{"file_id":"file","parent_file_id":"parent","name":"源文件.txt","type":"file","size":14,"content_hash_name":"sha1","content_hash":"` + hash + `"}`
		default:
			return nil, errors.New("非预期请求")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	id := q.enqueue("aliopen_test", "drive", "b:parent", "rename", source, "源文件.txt", 14).UploadID
	q.workers.Wait()
	if q.List()[0].Upload.Attempt.State != "unknown" {
		t.Fatal("丢失回执必须阻止直接重发")
	}
	if err := q.Resume(id); err != nil {
		t.Fatal(err)
	}
	if !q.List()[0].Upload.IsCompleted || creates != 1 {
		t.Fatalf("核对时重复上传: creates=%d %+v", creates, q.List())
	}
}
