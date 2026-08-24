package app

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive/providers/pikpak"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn appRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestProviderLoginFailureIsLoggedOnceAtOperationBoundary(t *testing.T) {
	pikpak.ResetPikPakLoginCooldown()
	t.Cleanup(pikpak.ResetPikPakLoginCooldown)

	previousTransport := netx.TestTransportHook
	netx.TestTransportHook = appRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"AccessProhibited"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { netx.TestTransportHook = previousTransport })

	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	_, err := NewApp().ProviderLogin(model.ProviderPikpak, map[string]string{
		"username":      "test@example.com",
		"password":      "secret",
		"captcha_token": "verified-token",
	})
	if err == nil {
		t.Fatal("ProviderLogin returned nil error for a provider risk-control response")
	}

	if got := strings.Count(output.String(), "level=WARN"); got != 1 {
		t.Fatalf("warning count = %d, want exactly one operation-boundary warning\nlogs:\n%s", got, output.String())
	}
	if !strings.Contains(output.String(), "msg=\"provider login failed\"") {
		t.Fatalf("operation-boundary warning is missing\nlogs:\n%s", output.String())
	}
}

func TestShouldPersistShareHistory(t *testing.T) {
	if shouldPersistShareHistory(&model.ShareItem{SharePolicy: "presigned"}) {
		t.Fatal("presigned URL must not be persisted as share history")
	}
	if !shouldPersistShareHistory(&model.ShareItem{SharePolicy: "public"}) {
		t.Fatal("provider-managed share should be persisted")
	}
}

func TestValidateShareRecordProvider(t *testing.T) {
	if err := validateShareRecordProvider(model.ShareHistoryEntry{}, model.ProviderDropbox); err != nil {
		t.Fatalf("legacy share record should remain usable: %v", err)
	}
	if err := validateShareRecordProvider(model.ShareHistoryEntry{Provider: model.ProviderDropbox}, model.ProviderDropbox); err != nil {
		t.Fatalf("matching provider should be accepted: %v", err)
	}
	if err := validateShareRecordProvider(model.ShareHistoryEntry{Provider: model.ProviderDropbox}, model.ProviderOnedrive); err == nil {
		t.Fatal("mismatched provider must be rejected before cancellation")
	}
}
