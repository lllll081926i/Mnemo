package model

import (
	"net/url"
	"strings"
)

func SafeOfflineDisplayURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[invalid URL]" || raw == "[unsupported URL]" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return "[invalid URL]"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Hostname() == "" {
			return "[invalid URL]"
		}
		return (&url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: parsed.Host}).String()
	case "magnet":
		query := url.Values{}
		for _, value := range parsed.Query()["xt"] {
			query.Add("xt", value)
		}
		return "magnet:?" + strings.ReplaceAll(query.Encode(), "%3A", ":")
	case "ed2k":
		return parsed.String()
	default:
		return "[unsupported URL]"
	}
}
