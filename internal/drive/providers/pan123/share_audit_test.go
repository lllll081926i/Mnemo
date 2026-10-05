package pan123

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestShareImportConsumesAllPagesAndRejectsRepeatedFiles(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		t.Run(fmt.Sprint(repeated), func(t *testing.T) {
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous })
			calls := 0
			netx.TestTransportHook = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Query().Get("Page") != fmt.Sprint(calls) {
					t.Fatalf("分享分页未推进: %s", request.URL)
				}
				fileID, next := calls, "0"
				if calls == 2 {
					next = "-1"
					if repeated {
						fileID = 1
					}
				}
				body := fmt.Sprintf(`{"code":0,"data":{"InfoList":[{"FileId":%d,"FileName":"file"}],"Next":"%s"}}`, fileID, next)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})
			files, err := pan123ShareList(context.Background(), &Driver{}, drive.Context{Token: &model.TokenInfo{AccessToken: "token"}}, "key", "pwd", "0")
			if repeated {
				if err == nil || files != nil {
					t.Fatalf("重复页不应当成完整列表: %v, %v", files, err)
				}
			} else if err != nil || len(files) != 2 || calls != 2 {
				t.Fatalf("分享遗漏后续页: %v, %v", files, err)
			}
		})
	}
}

func TestFreshFileLookupDoesNotUseOldProviderPool(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { netx.TestTransportHook = previous; drive.SetTokenResolver(nil) })
	account := drive.Context{UserID: "pan123_fresh-audit", DriveID: "pan123:fresh-audit", Token: &model.TokenInfo{AccessToken: "token"}}
	putPool(account, pan123File{FileID: "9999", FileName: "old", Size: 3})
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) { return account.Token, nil })
	calls := 0
	netx.TestTransportHook = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Scheme+"://"+request.URL.Host+request.URL.Path != apiFileDetail || request.URL.Query().Get("fileId") != "9999" {
			testContext.Fatalf("未请求真实详情: %s", request.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"FileId":9999,"FileName":"fresh","Size":9}}`)), Request: request}, nil
	})
	file, err := drive.GetFileFreshContext(testContext.Context(), account.UserID, account.DriveID, "9999")
	if err != nil || calls != 1 || file.Name != "fresh" || file.Size != 9 {
		testContext.Fatalf("校验读到了旧池数据: %+v, %v, calls=%d", file, err, calls)
	}
}
