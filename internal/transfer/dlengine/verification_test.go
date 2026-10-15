package dlengine

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/netx"
)

func TestDownloadVerifiesHashBeforePublishing(testContext *testing.T) {
	payload := []byte("经过校验的数据")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.ServeContent(writer, request, "file", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	for _, segmented := range []bool{false, true} {
		for _, algorithm := range []string{"md5", "sha1", "sha256"} {
			testContext.Run(algorithm+map[bool]string{true: "分段", false: "单流"}[segmented], func(testContext *testing.T) {
				expected, err := netx.HashReader(bytes.NewReader(payload), netx.HashKind(algorithm))
				if err != nil {
					testContext.Fatal(err)
				}
				path := filepath.Join(testContext.TempDir(), "file")
				if err := os.WriteFile(path, []byte("旧文件"), 0o600); err != nil {
					testContext.Fatal(err)
				}
				opts := Options{Concurrency: 2, ChunkSize: 4, MinSize: 1 << 20, ExpectedHash: strings.Repeat("0", len(expected)), HashAlgorithm: algorithm}
				if segmented {
					opts.MinSize = 1
				}
				if err := Download(testContext.Context(), opts, server.URL, path, nil); !errors.Is(err, ErrHashMismatch) {
					testContext.Fatalf("哈希不符被当作成功: %v", err)
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "旧文件" {
					testContext.Fatalf("校验失败覆盖旧目标: %s %v", data, err)
				}
				opts.ExpectedHash = strings.ToUpper(expected)
				if err := Download(testContext.Context(), opts, server.URL, path, nil); err != nil {
					testContext.Fatal(err)
				}
				data, err = os.ReadFile(path)
				if err != nil || !bytes.Equal(data, payload) {
					testContext.Fatalf("正确内容未发布: %s %v", data, err)
				}
			})
		}
	}
}

func TestContentHashRejectsCompositeAndUnknownDigests(testContext *testing.T) {
	for _, sample := range [][2]string{{"etag", strings.Repeat("a", 32)}, {"md5", strings.Repeat("a", 32) + "-2"}, {"dropbox", strings.Repeat("a", 64)}, {"quickxorhash", strings.Repeat("a", 40)}, {"sha256", "bad"}} {
		if ValidContentHash(sample[0], sample[1]) {
			testContext.Errorf("把非全文件摘要当作强校验: %v", sample)
		}
	}
}
