package driveutil

import (
	"errors"
	"net/url"
	"strings"
)

func ParseShareURL(raw string, domains ...string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Opaque != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("分享链接格式无效")
	}
	host := strings.ToLower(parsed.Hostname())
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return parsed, nil
		}
	}
	return nil, errors.New("分享链接域名与网盘不匹配")
}
