package webdav

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPutConditionFailureDoesNotFallBackToUnconditionalWrite(testContext *testing.T) {
	base, err := url.Parse("https://example.test/dav")
	if err != nil {
		testContext.Fatal(err)
	}
	requests := 0
	client := &Client{Endpoint: base.String(), base: base, HTTP: &http.Client{Transport: testUploadTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("If-Match") != `"old-version"` {
			testContext.Error("条件写入头丢失")
		}
		return &http.Response{StatusCode: 412, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	})}}
	err = client.Put(testContext.Context(), "/file", strings.NewReader("new"), 3, map[string]string{"If-Match": `"old-version"`})
	if err == nil || requests != 1 {
		testContext.Fatalf("条件失败不安全降级: requests=%d err=%v", requests, err)
	}
}
