package netx

import (
	"errors"
	"net/http"
	"strings"
)

// ErrHTTPSDowngrade indicates an upstream redirect attempted to downgrade an HTTPS request to HTTP.
var ErrHTTPSDowngrade = errors.New("netx: 拒绝从 HTTPS 降级重定向到 HTTP")

// SafeCheckRedirect enforces redirect safety for HTTP clients:
// 1. Caps redirect chains at 10 hops (matching net/http default).
// 2. Refuses HTTPS -> HTTP protocol downgrade (prevents credential/content exposure).
// 3. Strips sensitive auth headers when crossing host or scheme boundaries.
func SafeCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 || req == nil || req.URL == nil {
		return nil
	}
	prev := via[len(via)-1]
	if prev != nil && prev.URL != nil {
		if strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
			return ErrHTTPSDowngrade
		}
		if !sameOriginHost(prev.URL.Scheme, prev.URL.Host, req.URL.Scheme, req.URL.Host) {
			stripSensitiveHeaders(req.Header)
		}
	}
	return nil
}

func sameOriginHost(schemeA, hostA, schemeB, hostB string) bool {
	return strings.EqualFold(schemeA, schemeB) && strings.EqualFold(hostA, hostB)
}

func stripSensitiveHeaders(h http.Header) {
	if h == nil {
		return
	}
	for _, key := range []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"X-Auth-Token",
		"X-Access-Token",
		"X-Api-Key",
	} {
		h.Del(key)
	}
}
