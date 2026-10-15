package onedrive

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestSessionRecoversLostFinalResponseWithoutSecondPut(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { netx.TestTransportHook = previous })
	puts := 0
	netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
		switch request.Method {
		case http.MethodPost:
			return onedriveResponse(request, 200, `{"uploadUrl":"https://upload.example/session"}`), nil
		case http.MethodPut:
			puts++
			return nil, errors.New("完成响应丢失")
		case http.MethodGet:
			if request.Header.Get("Authorization") != "" {
				testContext.Error("上传会话查询泄露 Graph 凭据")
			}
			if puts == 0 {
				return onedriveResponse(request, 200, `{"nextExpectedRanges":["0-"]}`), nil
			}
			return onedriveResponse(request, 200, `{"id":"committed"}`), nil
		}
		return nil, errors.New("非预期请求")
	})
	path := filepath.Join(testContext.TempDir(), "file")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		testContext.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		testContext.Fatal(err)
	}
	defer file.Close()
	upload := &model.UploadingUI{}
	err = newClient("secret").sessionUpload(testContext.Context(), drive.Context{}, file, "root", "file", upload, "fail")
	if err != nil || puts != 1 || upload.Upload.FileID != "committed" {
		testContext.Fatalf("未知回执重发或没有对账: puts=%d upload=%+v err=%v", puts, upload, err)
	}
}

func TestRetryAfterIsCancelableAndLongCooldownNotIgnored(testContext *testing.T) {
	ctx, cancel := context.WithCancel(testContext.Context())
	cancel()
	if err := waitUploadRetry(ctx, "30", time.Second); !errors.Is(err, context.Canceled) {
		testContext.Fatalf("冷却不可取消: %v", err)
	}
	if err := waitUploadRetry(testContext.Context(), "301", time.Second); err == nil {
		testContext.Fatal("忽略服务端长时间冷却")
	}
}

func TestGraphPaginationRejectsForeignAudiences(testContext *testing.T) {
	for _, target := range []string{"https://tenant.sharepoint.com/path", "https://cdn.1drv.com/path", "https://user@graph.microsoft.com/v1.0/me", "https://graph.microsoft.com.evil.test/path", "http://graph.microsoft.com/path", "//evil.test/path"} {
		if trustedGraphURL(target) {
			testContext.Errorf("分页允许其他凭据受众: %s", target)
		}
		if err := newClient("secret").getJSON(testContext.Context(), target, nil); err == nil {
			testContext.Errorf("拒绝分页失败: %s", target)
		}
	}
}

func TestCopyMonitorDoesNotSendGraphTokenToSharepointOrAcceptUnknownSuccess(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "" {
			testContext.Error("Graph token 被发往 SharePoint monitor")
		}
		return onedriveResponse(request, 200, `{}`), nil
	})
	if _, err := newClient("secret").pollCopy(testContext.Context(), "https://tenant.sharepoint.com/_api/monitor/task"); err == nil {
		testContext.Fatal("没有完成状态的回执被当作成功")
	}
}
