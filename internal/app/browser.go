package app

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/logging"
)

// OpenBrowser opens a validated web URL in the system browser.
func (a *App) OpenBrowser(rawURL string) error {
	validated, err := validateExternalBrowserURL(rawURL)
	if err != nil {
		logging.Warn("external browser request rejected", "url_host", urlHost(rawURL), "error", err)
		return err
	}
	logging.Info("opening external browser", "url_host", validated.Hostname())
	if ctx, ok := a.wailsContext(); ok {
		runtime.BrowserOpenURL(ctx, validated.String())
		return nil
	}
	logging.Warn("external browser request skipped", "reason", "wails context unavailable")
	return errors.New("应用界面尚未初始化")
}

func validateExternalBrowserURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return nil, errors.New("外部链接无效")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return parsed, nil
	case "http":
		host := strings.ToLower(parsed.Hostname())
		ip := net.ParseIP(host)
		if host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return parsed, nil
		}
	}
	return nil, errors.New("仅允许 HTTPS 链接或本机 HTTP 回调")
}
