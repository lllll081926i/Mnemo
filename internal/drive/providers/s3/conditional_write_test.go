package s3

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestMultipartConditionFailureNeverRetriesWithoutCondition(testContext *testing.T) {
	previous := TransportOverride
	testContext.Cleanup(func() { TransportOverride = previous })
	commits := 0
	TransportOverride = uploadRoundTripper(func(request *http.Request) (*http.Response, error) {
		status := 200
		body := ""
		headers := make(http.Header)
		switch {
		case request.Method == http.MethodPost && request.URL.Query().Has("uploads"):
			body = `<InitiateMultipartUploadResult><UploadId>session</UploadId></InitiateMultipartUploadResult>`
		case request.Method == http.MethodPut:
			_, _ = io.Copy(io.Discard, request.Body)
			headers.Set("ETag", `"part"`)
		case request.Method == http.MethodPost:
			commits++
			if request.Header.Get("If-None-Match") != "*" {
				testContext.Error("完成分片丢失原子条件")
			}
			status = http.StatusPreconditionFailed
			body = `<Error><Code>PreconditionFailed</Code></Error>`
		default:
			testContext.Errorf("非预期请求: %s", request.Method)
		}
		return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
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
	account := drive.Context{Token: &model.TokenInfo{Conn: testS3Config()}}
	connection, err := connOf(account)
	if err != nil {
		testContext.Fatal(err)
	}
	err = uploadMultipart(testContext.Context(), account, connection, "file", file, &model.UploadingUI{Info: model.UploadInfo{Size: 3}}, nil, aws.String("*"))
	if err == nil || commits != 1 {
		testContext.Fatalf("条件失败被忽略或不安全重发: commits=%d err=%v", commits, err)
	}
}
