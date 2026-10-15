package onedrive

import (
	"errors"
	"fmt"

	"net/http"
	"os"
	"path/filepath"
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
			sessions := &failingCheckpointStore{failure: failure, failAt: failAt}
			drive.SetUploadSessionStore(sessions)
			puts := 0
			netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
				switch request.Method {
				case http.MethodPost:
					return onedriveResponse(request, 200, `{"uploadUrl":"https://upload.example/session"}`), nil
				case http.MethodGet:
					return onedriveResponse(request, 200, `{"nextExpectedRanges":["0-"]}`), nil
				case http.MethodPut:
					puts++
					return onedriveResponse(request, 202, `{"nextExpectedRanges":["3-"]}`), nil
				}
				return nil, fmt.Errorf("非预期请求 %s", request.Method)
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
			err = newClient("token").sessionUpload(testContext.Context(), drive.Context{}, file, "parent", "file", &model.UploadingUI{}, "replace")
			if !errors.Is(err, failure) || puts != failAt-1 {
				testContext.Fatalf("检查点错误未停止传输: puts=%d err=%v", puts, err)
			}
		})
	}
}
