package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// downloadTo streams a download url into a writer.
func downloadTo(ctx context.Context, dl *model.DownloadURL, w io.Writer) error {
	return downloadToCounted(ctx, dl, w, nil)
}

// downloadMigrationSource resolves a short-lived source URL and streams it to
// a migration destination. A signed URL or an OAuth bearer can expire in the
// small gap between resolution and the actual transfer. Retry that exact,
// recoverable class once by asking the provider for a fresh URL; all other
// errors keep their first result and never create retry traffic.
func downloadMigrationSource(ctx context.Context, job *Job, srcFile *model.File, w io.Writer) error {
	if job == nil || srcFile == nil {
		return errors.New("migrate: source job or file is empty")
	}
	resolve := func() (*model.DownloadURL, error) {
		return drive.GetDownloadURLContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID, 3600)
	}
	dl, err := resolve()
	if err != nil {
		return fmt.Errorf("migrate: resolve download url: %w", err)
	}
	err = downloadToCounted(ctx, dl, w, job)
	if !retryableMigrationDownloadAuthFailure(err) {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	dl, refreshErr := resolve()
	if refreshErr != nil {
		return fmt.Errorf("migrate: refresh download url after authentication failure: %w", refreshErr)
	}
	if retryErr := downloadToCounted(ctx, dl, w, job); retryErr != nil {
		return fmt.Errorf("migrate: refreshed download url still failed: %w", retryErr)
	}
	return nil
}

// downloadToCounted is like downloadTo but accumulates the byte count into
// job.ProcessedBytes when job is non-nil.
func downloadToCounted(ctx context.Context, dl *model.DownloadURL, w io.Writer, job *Job) error {
	if dl == nil || strings.TrimSpace(dl.URL) == "" {
		return errors.New("migrate: empty download url")
	}
	hc := netx.NewClient(0) // honors global proxy; no total timeout for large files
	req, err := hc.Req(ctx, http.MethodGet, dl.URL, nil)
	if err != nil {
		return err
	}
	for key, value := range dl.Headers {
		req.Header.Set(key, value)
	}
	if dl.RequestAuth != nil {
		if err := dl.RequestAuth(req); err != nil {
			return err
		}
	}
	resp, err := hc.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return migrationHTTPError(resp)
	}
	var counter io.Writer = w
	if job != nil {
		counter = &countingWriter{w: w, job: job, emit: func() {}}
	}
	_, err = io.Copy(counter, resp.Body)
	return err
}

// migrationHTTPError preserves the small, actionable part of an upstream
// download failure.  A migration otherwise loses the provider's response body
// after resolving a direct URL, making a token/region/path failure look like
// an unhelpful generic HTTP status in the task list.
type migrationDownloadHTTPError struct {
	statusCode int
	detail     string
	requestID  string
}

func (e *migrationDownloadHTTPError) Error() string {
	if e == nil {
		return "migrate: empty http response"
	}
	message := fmt.Sprintf("migrate: download http %d", e.statusCode)
	if e.detail != "" {
		message += ": " + e.detail
	}
	if e.requestID != "" {
		message += " (request_id=" + e.requestID + ")"
	}
	return message
}

func retryableMigrationDownloadAuthFailure(err error) bool {
	var httpErr *migrationDownloadHTTPError
	return errors.As(err, &httpErr) && (httpErr.statusCode == http.StatusUnauthorized || httpErr.statusCode == http.StatusForbidden)
}

func migrationHTTPError(resp *http.Response) error {
	const maxDetailBytes = 2048
	if resp == nil {
		return errors.New("migrate: empty http response")
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDetailBytes+1))
	truncated := len(body) > maxDetailBytes
	if truncated {
		body = body[:maxDetailBytes]
	}
	detail := strings.Join(strings.Fields(string(body)), " ")
	if truncated && detail != "" {
		detail += "…"
	}

	requestID := firstHeader(resp.Header,
		"X-Dropbox-Request-Id",
		"X-Request-Id",
		"Request-Id",
		"X-Reqid",
	)
	return &migrationDownloadHTTPError{
		statusCode: resp.StatusCode,
		detail:     detail,
		requestID:  requestID,
	}
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

// countingWriter wraps a writer and increments job.ProcessedBytes.
type countingWriter struct {
	w    io.Writer
	job  *Job
	emit func()
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.job.ProcessedBytes += int64(n)
	}
	return n, err
}

// completeBytes commits exactly one file's worth of progress after its target
// upload succeeds. Download strategies may have already reported incremental
// bytes, so assigning the final value avoids double-counting on success.
func completeBytes(job *Job, start, size int64) {
	if job == nil {
		return
	}
	job.ProcessedBytes = start + migrationProgressSize(size)
}
