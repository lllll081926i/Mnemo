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
	if got := sanitizeText("GET https://example.test/callback?token=secret123"); got != "GET https://example.test/callback" {
		t.Fatalf("sanitized text = %q", got)
	}
	if got := sanitizeText("pikpak: captcha_required\nurl=https://user.mypikpak.com/captcha/v2/txCaptcha.html?action=POST&creditkey=secret"); got != "pikpak: captcha_required url=https://user.mypikpak.com/captcha/v2/txCaptcha.html" {
		t.Fatalf("sanitized challenge = %q", got)
	}
}
