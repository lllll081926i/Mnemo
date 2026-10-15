package pan189

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
			previous := netx.TestTransportHook
			testContext.Cleanup(func() { netx.TestTransportHook = previous; drive.SetUploadSessionStore(nil) })
			failure := errors.New("模拟磁盘写入失败")
			drive.SetUploadSessionStore(&failingCheckpointStore{failure: failure, failAt: failAt})
			puts, commits := 0, 0
			netx.TestTransportHook = pan189AuthRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				body := "{}"
				switch {
				case request.Method == http.MethodPut:
					puts++
				case strings.HasSuffix(request.URL.Path, "/initMultiUpload"):
					body = `{"data":{"uploadFileId":"session","fileDataExists":0}}`
				case strings.HasSuffix(request.URL.Path, "/getMultiUploadUrls"):
					body = `{"uploadUrls":{"partNumber_1":{"requestURL":"https://upload.example/part"}}}`
				case strings.HasSuffix(request.URL.Path, "/commitMultiUploadFile"):
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
			session := &Session{SessionKey: "key", SessionSecret: "0123456789abcdef"}
			account := drive.Context{UserID: "pan189:test", DriveID: "drive", Token: &model.TokenInfo{AccessToken: "key", RefreshToken: mustJSON(session)}}
			err := (&Driver{}).UploadOneFile(testContext.Context(), account, &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, ParentFileID: "root", Name: "file.txt"}})
			if !errors.Is(err, failure) || puts != failAt-1 || commits != 0 {
				testContext.Fatalf("检查点错误未停止传输: puts=%d commits=%d err=%v", puts, commits, err)
			}
		})
	}
}
