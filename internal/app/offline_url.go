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
		q := parsed.Query()
		for key, values := range q {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") ||
				strings.Contains(lower, "secret") ||
				strings.Contains(lower, "sign") ||
				strings.Contains(lower, "auth") ||
				strings.Contains(lower, "credential") ||
				strings.Contains(lower, "password") ||
				strings.Contains(lower, "passwd") ||
				strings.Contains(lower, "policy") ||
				strings.Contains(lower, "key") ||
				strings.HasPrefix(lower, "x-amz-") ||
				strings.HasPrefix(lower, "x-oss-") {
				for i := range values {
					values[i] = "[redacted]"
				}
				q[key] = values
			}
		}
		parsed.RawQuery = q.Encode()
		return parsed.String()
	case "magnet":
		// A magnet's xt=urn:btih identifies the resource; it is not a
		// presigned HTTP credential. Drop trackers if present.
		q := parsed.Query()
		q.Del("tr")
		q.Del("ws")
		parsed.RawQuery = q.Encode()
		return parsed.String()
	case "ed2k":
		return parsed.String()
	default:
		return "[unsupported URL]"
	}
}
