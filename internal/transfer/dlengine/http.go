package dlengine

import (
	"net/http"
	"net/url"

	"mnemo-go/internal/netx"
)

func setHeaders(req *http.Request, headers map[string]string, ua string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
}

func setRequestHeaders(req *http.Request, opts Options) error {
	setHeaders(req, opts.Headers, opts.UserAgent)
	if opts.RequestAuth != nil {
		return opts.RequestAuth(req)
	}
	return nil
}

// proxyFunc returns a proxy function that honors the netx global proxy first,
// falling back to the environment (HTTP_PROXY/HTTPS_PROXY).
func proxyFunc() func(*http.Request) (*url.URL, error) {
	gp := netx.GlobalProxy()
	if gp != "" {
		if u, err := url.Parse(gp); err == nil && u.Scheme != "" {
			return http.ProxyURL(u)
		}
	}
	return http.ProxyFromEnvironment
}
