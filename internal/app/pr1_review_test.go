package app

import (
	"strings"
	"testing"
)

func TestPR1ReviewOfflineSignatureIsNotPersisted(testContext *testing.T) {
	result := safeOfflineDisplayURL("https://example.test/file?sig=review-secret&sp=r&se=2099-01-01")
	if strings.Contains(result, "review-secret") {
		testContext.Fatalf("签名仍进入显示及持久化 URL: %s", result)
	}
}
