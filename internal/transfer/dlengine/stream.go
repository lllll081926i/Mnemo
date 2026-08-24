package dlengine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func singleStream(ctx context.Context, hc *http.Client, opts Options, url, localPath string, total int64, validator resourceValidator, onProgress func(Progress)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if err := setRequestHeaders(req, opts); err != nil {
		return err
	}
	validator.setFullRequestCondition(req)
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return errResourceChanged
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dlengine: http %d", resp.StatusCode)
	}
	if err := validator.verifyResponse(resp.Header); err != nil {
		return err
	}
	partPath := localPath + ".part"
	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	var limiter RateLimiter = opts.Limiter
	if limiter == nil {
		limiter = newSpeedLimiter(opts.MaxSpeed)
	}
	buf := make([]byte, 256<<10)
	var written int64
	last := time.Now()
	speed := newSpeedEstimator(last, 0)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if err := limiter.Wait(ctx, int64(n)); err != nil {
				return err
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if onProgress != nil && time.Since(last) >= progressInterval {
				now := time.Now()
				last = now
				onProgress(Progress{Downloaded: written, Total: total, Speed: speed.Observe(now, written), Percent: percent(written, total)})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if total > 0 && written != total {
		return fmt.Errorf("dlengine: downloaded %d != total %d", written, total)
	}
	if err := commitPart(partPath, localPath); err != nil {
		return err
	}
	_ = os.Remove(localPath + ".state.json")
	if onProgress != nil {
		onProgress(Progress{Downloaded: written, Total: total, Speed: 0, Percent: percent(written, total)})
	}
	return nil
}
