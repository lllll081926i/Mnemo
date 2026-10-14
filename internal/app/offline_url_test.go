package app

import (
	"strings"
	"testing"
)

func TestSafeOfflineDisplayURLRedactsCredentials(t *testing.T) {
	raw := "https://user:password@example.test/download?file=ok&X-Amz-Signature=secret123&access_token=abcdef#private"
	got := safeOfflineDisplayURL(raw)
	for _, secret := range []string{"user:password", "secret123", "abcdef", "#private"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sensitive credential in persisted task URL: %q", got)
		}
	}
	if got != "https://example.test/download" {
		t.Fatalf("redaction destroyed harmless resource metadata: %q", got)
	}
	if safeOfflineDisplayURL("magnet:?xt=urn:btih:hash") != "magnet:?xt=urn:btih:hash" {
		t.Fatal("ordinary magnet links must retain their original representation")
	}
}
