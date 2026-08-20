package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSanitizeArgsRedactsSecrets(t *testing.T) {
	args := sanitizeArgs("error", errors.New("captcha_token=secret123 password=hidden"), "has_token", true)
	if got := args[1].(string); got != "captcha_token=[REDACTED] password=[REDACTED]" {
		t.Fatalf("error attribute = %v, want sanitized", got)
	}
	if got := args[3]; got != true {
		t.Fatalf("safe token presence metadata = %v, want true", got)
	}
	secret := sanitizeArgs("access_token", "secret123")
	if got := secret[1]; got != "[REDACTED]" {
		t.Fatalf("access token = %v, want redacted", got)
	}
	if got := sanitizeText("GET https://example.test/callback?token=secret123"); got != "GET https://example.test/callback" {
		t.Fatalf("sanitized text = %q", got)
	}
	if got := sanitizeText("pikpak: captcha_required\nurl=https://user.mypikpak.com/captcha/v2/txCaptcha.html?action=POST&creditkey=secret"); got != "pikpak: captcha_required url=https://user.mypikpak.com/captcha/v2/txCaptcha.html" {
		t.Fatalf("sanitized challenge = %q", got)
	}
}

func TestCompactHandlerStartsWithLevelAndKeepsDiagnosticFields(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.FixedZone("HKT", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocal })

	var output bytes.Buffer
	var testLevel slog.LevelVar
	testLevel.Set(slog.LevelDebug)
	handler := &compactHandler{writer: &output, level: &testLevel}
	record := slog.NewRecord(time.Date(2026, 8, 20, 12, 34, 56, 123000000, time.FixedZone("HKT", 8*60*60)), slog.LevelWarn, "connection validation failed", 0)
	record.Add("provider", "webdav", "status", 530, "error", errors.New("password=secret endpoint=https://dav.example.test/root?token=secret"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	line := output.String()
	if !strings.HasPrefix(line, "[WARN] connection validation failed | ") {
		t.Fatalf("unexpected log prefix: %q", line)
	}
	for _, want := range []string{"time=2026-08-20T12:34:56.123+08:00", "provider=webdav", "status=530", "password=[REDACTED]", "https://dav.example.test/root"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line missing %q: %s", want, line)
		}
	}
	for _, unwanted := range []string{"level=WARN", "msg=", "secret"} {
		if strings.Contains(line, unwanted) {
			t.Errorf("log line contains %q: %s", unwanted, line)
		}
	}
}
