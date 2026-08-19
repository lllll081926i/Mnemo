package logging

import (
	"errors"
	"testing"
)

func TestSanitizeArgsRedactsSecrets(t *testing.T) {
	args := sanitizeArgs("error", errors.New("captcha_token=secret123 password=hidden"), "has_token", true)
	if got := args[1].(string); got != "captcha_token=[REDACTED] password=[REDACTED]" {
		t.Fatalf("error attribute = %v, want sanitized", got)
	}
	if got := args[3]; got != "[REDACTED]" {
		t.Fatalf("token metadata = %v, want redacted", got)
	}
	if got := sanitizeText("GET https://example.test/callback?token=secret123"); got != "GET https://example.test/callback?token=[REDACTED]" {
		t.Fatalf("sanitized text = %q", got)
	}
}
