package aliopen

import (
	"errors"
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

type failingCheckpointStore struct {
	failure error
	failAt  int
	saves   int
}

func (storage *failingCheckpointStore) SaveUploadSession(string, []int) error {
	storage.saves++
	if storage.saves == storage.failAt {
		return storage.failure
	}
	return nil
}
func (*failingCheckpointStore) LoadUploadSession(string) []int { return nil }
func (*failingCheckpointStore) ClearUploadSession(string)      {}
func TestUploadCheckpointFailureStopsTransfer(testContext *testing.T) {
	for _, failAt := range []int{1, 2} {
		testContext.Run(fmt.Sprint(failAt), func(testContext *testing.T) {
			previous, limiter := netx.TestTransportHook, aliOpenLimiter
			aliOpenLimiter = newAliOpenRateLimiter(2, 0)
			testContext.Cleanup(func() { netx.TestTransportHook = previous; aliOpenLimiter = limiter; drive.SetUploadSessionStore(nil) })
			failure := errors.New("模拟磁盘写入失败")
			drive.SetUploadSessionStore(&failingCheckpointStore{failure: failure, failAt: failAt})
			puts, commits := 0, 0
			netx.TestTransportHook = aliOpenRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				body := "{}"
				switch {
				case request.Method == http.MethodPut:
					puts++
				case request.URL.Path == "/adrive/v1.0/openFile/create":
					body = `{"file_id":"file","upload_id":"session","part_info_list":[{"part_number":1,"upload_url":"https://upload.example/part"}]}`
				case request.URL.Path == "/adrive/v1.0/openFile/complete":
					commits++
				default:
					return nil, fmt.Errorf("非预期请求 %s", request.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})
			path := filepath.Join(testContext.TempDir(), "file")
			if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
				testContext.Fatal(err)
			}
			account := drive.Context{UserID: "aliopen:test", DriveID: "drive", Token: &model.TokenInfo{RefreshToken: mustJSON(Session{AccessToken: "token", DriveID: "drive"})}}
			err := (&Driver{}).UploadOneFile(testContext.Context(), account, &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, ParentFileID: "b:root", Name: "file.txt"}})
			if !errors.Is(err, failure) || puts != failAt-1 || commits != 0 {
				testContext.Fatalf("检查点错误未停止传输: puts=%d commits=%d err=%v", puts, commits, err)
			}
		})
	}
}
