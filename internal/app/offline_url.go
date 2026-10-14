package app

import (
	"net/url"
	"strings"
)

// safeOfflineDisplayURL removes credentials from a persisted offline task.
// The original link is sent only to the selected provider for task creation.
// Cloud signed URLs may contain bearer tokens in query parameters, and the
// task history does not need these values to display provider progress.
func safeOfflineDisplayURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return "[invalid URL]"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http":
		if parsed.Hostname() == "" {
			return "[invalid URL]"
		}
		parsed.User = nil
		parsed.Fragment = ""
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		return parsed.String()
	case "magnet":
		// A magnet's xt=urn:btih identifies the resource; it is not a
		// presigned HTTP credential. Drop trackers if present.
		q := parsed.Query()
		if q.Has("tr") || q.Has("ws") {
			q.Del("tr")
			q.Del("ws")
			parsed.RawQuery = q.Encode()
			return parsed.String()
		}
		return strings.TrimSpace(raw)
	case "ed2k":
		return parsed.String()
	default:
		return "[unsupported URL]"
	}
}
