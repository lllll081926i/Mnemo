package driveutil

import (
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// amzDateRe matches AWS X-Amz-Date values like 20250101T000000Z.
var amzDateRe = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z?$`)

// GetExpiresTime extracts the unix-millis expiration embedded in a signed
// download URL (port of the legacy frontend GetExpiresTime util). Returns 0
// when no expiration parameter is present or the URL is not parseable.
func GetExpiresTime(downURL string) int64 {
	raw := downURL
	if dec, err := url.QueryUnescape(downURL); err == nil {
		// decodeURIComponent in the legacy impl throws on invalid UTF-8 and
		// the whole helper returns 0; mirror that strictness.
		if utf8.ValidString(dec) {
			raw = dec
		} else {
			return 0
		}
	}
	if raw == "" {
		return 0
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	// Query parameter names are case-insensitive (AWS style X-Amz-Date is
	// uppercase); collect one value per lowercased key.
	params := map[string]string{}
	for k, vs := range parsed.Query() {
		lower := strings.ToLower(k)
		if _, ok := params[lower]; !ok && len(vs) > 0 {
			params[lower] = vs[0]
		}
	}
	// AWS signature URLs (PikPak dl-xxx, mounted S3):
	// validity = X-Amz-Date + X-Amz-Expires seconds.
	if amzDate := params["x-amz-date"]; amzDate != "" {
		amzExpires := params["x-amz-expires"]
		if n, ok := parseFiniteNum(amzExpires); ok && n > 0 {
			if m := amzDateRe.FindStringSubmatch(amzDate); m != nil {
				base := time.Date(atoi(m[1]), time.Month(atoi(m[2])), atoi(m[3]),
					atoi(m[4]), atoi(m[5]), atoi(m[6]), 0, time.UTC)
				if !base.IsZero() {
					return base.UnixMilli() + int64(n)*1000
				}
			}
		}
	}
	for _, key := range []string{"x-oss-expires", "expire", "expires", "expires_at", "exp", "e"} {
		value := params[key]
		if value == "" {
			continue
		}
		if n, ok := parseFiniteNum(value); ok {
			// Pure numeric: second timestamps must be >= 2001 (1e9) so that
			// business params like ?e=1 are not misread as dates.
			if n >= 1_000_000_000 {
				if n < 10_000_000_000 {
					return int64(n) * 1000
				}
				return int64(n)
			}
			continue
		}
		if t := parseDateString(value); t > 0 {
			return t
		}
	}
	return 0
}

func parseFiniteNum(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

func parseDateString(s string) int64 {
	layouts := []string{
		time.RFC3339,
		time.RFC1123,
		time.RFC1123Z,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006/01/02",
		"Jan 2 2006 15:04:05 GMT-0700",
		"Mon, 02 Jan 2006 15:04:05 GMT-0700",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
