package dlengine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// probe determines the file size, range support and resource validators.
func probe(ctx context.Context, hc *http.Client, opts Options, url string) (int64, bool, resourceValidator, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, resourceValidator{}, err
	}
	if err := setRequestHeaders(req, opts); err != nil {
		return 0, false, resourceValidator{}, err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := hc.Do(req)
	if err != nil {
		return 0, false, resourceValidator{}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	// A server advertising Accept-Ranges can still ignore a particular probe.
	// Only a real 206 response proves that segmented requests are usable.
	acceptRanges := resp.StatusCode == http.StatusPartialContent
	validator := resourceValidator{
		ETag:         strings.TrimSpace(resp.Header.Get("ETag")),
		LastModified: strings.TrimSpace(resp.Header.Get("Last-Modified")),
	}
	var total int64
	switch {
	case resp.StatusCode == http.StatusPartialContent:
		start, end, parsedTotal, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != 0 || end != 0 {
			return 0, false, resourceValidator{}, fmt.Errorf("%w: invalid probe Content-Range", errInvalidRangeResponse)
		}
		if resp.ContentLength >= 0 && resp.ContentLength != 1 {
			return 0, false, resourceValidator{}, fmt.Errorf("%w: probe Content-Length is %d, want 1", errInvalidRangeResponse, resp.ContentLength)
		}
		total = parsedTotal
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		total = resp.ContentLength
	default:
		return 0, false, resourceValidator{}, fmt.Errorf("dlengine: probe http %d", resp.StatusCode)
	}
	if total <= 0 {
		// Chunked CDN responses legitimately omit Content-Length. They cannot
		// be resumed or split safely, but a regular single-stream download is
		// still valid. The caller may supply ExpectedSize for progress display.
		return 0, false, validator, nil
	}
	return total, acceptRanges, validator, nil
}

func parseContentRange(value string) (start, end, total int64, ok bool) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bytes") {
		return 0, 0, 0, false
	}
	rangeAndTotal := strings.SplitN(fields[1], "/", 2)
	if len(rangeAndTotal) != 2 || rangeAndTotal[1] == "*" {
		return 0, 0, 0, false
	}
	bounds := strings.SplitN(rangeAndTotal[0], "-", 2)
	if len(bounds) != 2 {
		return 0, 0, 0, false
	}
	start, errStart := strconv.ParseInt(bounds[0], 10, 64)
	end, errEnd := strconv.ParseInt(bounds[1], 10, 64)
	total, errTotal := strconv.ParseInt(rangeAndTotal[1], 10, 64)
	if errStart != nil || errEnd != nil || errTotal != nil || start < 0 || end < start || total <= 0 || end >= total {
		return 0, 0, 0, false
	}
	return start, end, total, true
}

func resumeIdentityMatches(previous state, current resourceValidator) bool {
	if previous.ETag != "" || current.ETag != "" {
		return previous.ETag != "" && current.ETag != "" && previous.ETag == current.ETag
	}
	if previous.LastModified != "" || current.LastModified != "" {
		return previous.LastModified != "" && current.LastModified != "" && previous.LastModified == current.LastModified
	}
	return true
}

func strongETag(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 || strings.HasPrefix(strings.ToUpper(value), "W/") || value[0] != '"' || value[len(value)-1] != '"' {
		return ""
	}
	return value
}

func (v resourceValidator) ifRange() string {
	if etag := strongETag(v.ETag); etag != "" {
		return etag
	}
	return v.LastModified
}

func (v resourceValidator) setFullRequestCondition(req *http.Request) {
	if etag := strongETag(v.ETag); etag != "" {
		req.Header.Set("If-Match", etag)
		return
	}
	if v.LastModified != "" {
		req.Header.Set("If-Unmodified-Since", v.LastModified)
	}
}

func (v resourceValidator) verifyResponse(header http.Header) error {
	if v.ETag != "" {
		if responseETag := strings.TrimSpace(header.Get("ETag")); responseETag != "" && responseETag != v.ETag {
			return fmt.Errorf("%w: ETag no longer matches", errResourceChanged)
		}
	}
	if v.LastModified != "" {
		if responseModified := strings.TrimSpace(header.Get("Last-Modified")); responseModified != "" && responseModified != v.LastModified {
			return fmt.Errorf("%w: Last-Modified no longer matches", errResourceChanged)
		}
	}
	return nil
}
