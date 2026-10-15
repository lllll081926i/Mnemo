package model

import "testing"

func TestOfflineDisplayURLDropsCredentialsAndIsStable(testContext *testing.T) {
	for _, example := range []struct{ raw, expected string }{
		{"https://user:secret@example.test/private/secret?sig=secret#secret", "https://example.test"},
		{"http://example.test:8080/path%3Ftoken%3Dsecret", "http://example.test:8080"},
		{"magnet:?xt=urn:btih:abc&tr=https%3A%2F%2Ftracker.test%2F%3Ftoken%3Dsecret&ws=secret&custom=secret", "magnet:?xt=urn:btih:abc"},
		{"https:///missing-host", "[invalid URL]"},
		{"[unsupported URL]", "[unsupported URL]"},
		{"", ""},
	} {
		actual := SafeOfflineDisplayURL(example.raw)
		if actual != example.expected || SafeOfflineDisplayURL(actual) != actual {
			testContext.Fatalf("脱敏结果或重复处理错误: %q -> %q", example.raw, actual)
		}
	}
}
