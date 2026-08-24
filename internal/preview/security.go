package preview

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mnemo-go/internal/netx"
)

func isSafeProxyURL(raw string) bool {
	return isSafeProxyURLWithAllow(raw, nil)
}

// filterProxyHeaders drops hop-by-hop and obviously sensitive headers the
// frontend should not be able to inject into upstream requests.

func checkProxyRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if !isSafeProxyURL(req.URL.String()) {
		return fmt.Errorf("redirect target not allowed")
	}
	return nil
}

func (s *Server) checkProxyRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if err := s.validateSafeProxyURL(req.Context(), req.URL.String()); err != nil {
		return fmt.Errorf("redirect target not allowed: %w", err)
	}
	return nil
}

func proxyHostKey(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func (s *Server) rememberProxyHost(raw string, allowPrivate ...bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return
	}
	privateOptIn := len(allowPrivate) > 0 && allowPrivate[0]
	if !privateOptIn && !isExplicitLocalProxyHost(u.Hostname()) {
		return
	}
	key := proxyHostKey(u)
	s.mu.Lock()
	if s.allowedProxyHosts == nil {
		s.allowedProxyHosts = make(map[string]struct{})
	}
	s.allowedProxyHosts[key] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) isAllowedProxyHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false
	}
	key := proxyHostKey(u)
	s.mu.Lock()
	_, ok := s.allowedProxyHosts[key]
	s.mu.Unlock()
	return ok
}

func globalProxy(_ *http.Request) (*url.URL, error) {
	raw := strings.TrimSpace(netx.GlobalProxy())
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		if err == nil {
			err = fmt.Errorf("invalid proxy URL")
		}
		return nil, err
	}
	return u, nil
}

func (s *Server) isSafeProxyURL(raw string) bool {
	return isSafeProxyURLWithAllow(raw, s.isAllowedProxyHost)
}

func (s *Server) validateSafeProxyURL(ctx context.Context, raw string) error {
	if !s.isSafeProxyURL(raw) {
		return fmt.Errorf("proxy target is not allowed")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("proxy target is invalid")
	}
	if s.isAllowedProxyHost(raw) {
		return nil
	}
	if err := validateResolvedProxyHost(ctx, u.Hostname()); err != nil {
		return err
	}
	return nil
}

func (s *Server) safeProxyDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if isExplicitLocalProxyHost(host) || s.isAllowedProxyHostHostPort(host, port) {
		return (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, address)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		if isBlockedProxyIP(ip.IP) {
			lastErr = fmt.Errorf("proxy target resolved to a private address")
			continue
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("proxy target has no usable address")
	}
	return nil, lastErr
}

func (s *Server) isAllowedProxyHostHostPort(host, port string) bool {
	key := strings.ToLower(strings.TrimSpace(host))
	if port != "" {
		key = net.JoinHostPort(key, port)
	}
	s.mu.Lock()
	_, ok := s.allowedProxyHosts[key]
	s.mu.Unlock()
	return ok
}

func validateResolvedProxyHost(ctx context.Context, host string) error {
	if isExplicitLocalProxyHost(host) {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedProxyIP(ip) {
			return fmt.Errorf("proxy target is a private address")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("proxy target DNS lookup failed: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("proxy target has no address")
	}
	for _, ip := range ips {
		if isBlockedProxyIP(ip.IP) {
			return fmt.Errorf("proxy target resolves to a private address")
		}
	}
	return nil
}

func isBlockedProxyIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

func isExplicitLocalProxyHost(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	if ip := net.ParseIP(lower); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func isSafeProxyURLWithAllow(raw string, allow func(string) bool) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return allow != nil && allow(raw)
		}
		return true
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") ||
		strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".localhost") {
		return allow != nil && allow(raw)
	}
	return true
}

func isLocalOrigin(raw string) bool {
	if raw == "null" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "wails.localhost" || host == "127.0.0.1" || host == "::1"
}

// sanitizeDispositionFilename strips characters that are invalid inside a
// Content-Disposition filename quoted-string (RFC 6266). It keeps CJK and
// common punctuation but drops quotes, backslashes and control bytes.
