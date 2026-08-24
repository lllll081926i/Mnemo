package dlengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// fetchRange GETs one range and writes it at the file offset.
func fetchRange(ctx context.Context, hc *http.Client, opts Options, url string, start, length, total int64, validator resourceValidator, f *os.File, limiter RateLimiter, account func(int64)) error {
	if start < 0 || length <= 0 || total <= 0 || start > total-length {
		return errors.New("dlengine: invalid range length")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if err := setRequestHeaders(req, opts); err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+length-1))
	if condition := validator.ifRange(); condition != "" {
		req.Header.Set("If-Range", condition)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dlengine: range http %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		if validator.ifRange() != "" {
			return fmt.Errorf("%w: server returned the full resource after If-Range", errResourceChanged)
		}
		return errors.New("dlengine: server ignored range request")
	}
	expectedEnd := start + length - 1
	gotStart, gotEnd, gotTotal, ok := parseContentRange(resp.Header.Get("Content-Range"))
	if !ok || gotStart != start || gotEnd > expectedEnd || gotTotal != total {
		return fmt.Errorf("%w: requested bytes %d-%d/%d", errInvalidRangeResponse, start, expectedEnd, total)
	}
	responseLength := gotEnd - gotStart + 1
	if resp.ContentLength >= 0 && resp.ContentLength != responseLength {
		return fmt.Errorf("%w: Content-Length is %d, want %d", errInvalidRangeResponse, resp.ContentLength, responseLength)
	}
	if err := validator.verifyResponse(resp.Header); err != nil {
		return err
	}
	buf := make([]byte, 256<<10)
	written := int64(0)
	committed := false
	preservePartial := false
	defer func() {
		if !committed && !preservePartial && written > 0 && account != nil {
			// A retry from the original offset overwrites the same range. Roll
			// back bytes reported by this failed attempt.
			account(-written)
		}
	}()
	body := io.LimitReader(resp.Body, responseLength)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if err := limiter.Wait(ctx, int64(n)); err != nil {
				return err
			}
			if _, werr := f.WriteAt(buf[:n], start+written); werr != nil {
				return werr
			}
			written += int64(n)
			if account != nil {
				account(int64(n))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if written > 0 {
				preservePartial = true
				return &partialRangeReadError{err: err, written: written}
			}
			return err
		}
	}
	if written != responseLength {
		err := fmt.Errorf("dlengine: short read %d != %d", written, responseLength)
		if written > 0 {
			preservePartial = true
			return &partialRangeReadError{err: err, written: written}
		}
		return err
	}
	if responseLength < length {
		// Some CDNs cap an otherwise valid requested range. Continue from the
		// exact next byte instead of retrying the original range or waiting for
		// a backoff interval.
		preservePartial = true
		return &partialRangeReadError{err: errShortRangeResponse, written: written}
	}
	committed = true
	return nil
}
