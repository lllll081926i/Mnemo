// Package dlengine implements a native multi-connection segmented downloader
// (aria2-style). It replaces the external aria2c.exe dependency: files are
// fetched with parallel HTTP Range requests and assembled at offsets, with
// resume support, per-chunk retry, global speed limiting and progress events.
package dlengine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Defaults mirroring sane aria2 behaviour.
const (
	DefaultConcurrency = 8
	DefaultChunkSize   = 16 << 20 // 16 MiB per chunk
	DefaultMinSize     = 32 << 20 // below this: single stream
	MaxRetriesPerChunk = 5
	progressInterval   = 500 * time.Millisecond
	speedSampleWindow  = 2 * time.Second
	// MaxPartialRangeRequests prevents a broken server that returns only a
	// handful of bytes per response from causing an unbounded request loop.
	MaxPartialRangeRequests = 64
)

// Options tunes the download behaviour.
type Options struct {
	Concurrency int   // parallel connections
	ChunkSize   int64 // range chunk size
	MinSize     int64 // files smaller than this use a single stream
	MaxSpeed    int64 // global speed cap in bytes/s (0 = unlimited)
	// ExpectedSize is optional provider metadata. It is used only when the
	// download server omits Content-Length, so a chunked response can still
	// report progress and be verified after a single-stream download.
	ExpectedSize int64
	Limiter      RateLimiter
	Headers      map[string]string
	UserAgent    string
	// RequestAuth runs after static headers are applied, once per outbound
	// request. It is used for request-bound schemes such as HTTP Digest.
	RequestAuth func(*http.Request) error
}

// RateLimiter is shared by concurrent downloads so MaxDownloadSpeed is a
// process-wide cap rather than an independent bucket per task.
type RateLimiter interface {
	Wait(ctx context.Context, n int64) error
}

// Normalize fills zero fields with defaults.
func (o *Options) Normalize() {
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultConcurrency
	}
	if o.ChunkSize <= 0 {
		o.ChunkSize = DefaultChunkSize
	}
	if o.MinSize <= 0 {
		o.MinSize = DefaultMinSize
	}
	if o.UserAgent == "" {
		o.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Mnemo/1.0"
	}
}

// Progress is a periodic snapshot.
type Progress struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
	Speed      int64 `json:"speed"` // bytes/s
	Percent    int   `json:"percent"`
}

// state is the persisted resume state.
type state struct {
	URL          string `json:"url,omitempty"` // legacy read-only; never written
	URLHash      string `json:"url_hash,omitempty"`
	Total        int64  `json:"total"`
	Chunk        int64  `json:"chunk"`
	Done         []bool `json:"done"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

type resourceValidator struct {
	ETag         string
	LastModified string
}

var (
	errResourceChanged      = errors.New("dlengine: remote resource changed")
	errInvalidRangeResponse = errors.New("dlengine: invalid range response")
	errShortRangeResponse   = errors.New("dlengine: server returned a shorter valid range")
)

// partialRangeReadError reports bytes that were safely written before the
// remote side ended a valid range response. The caller can continue from the
// next byte instead of discarding the whole chunk and downloading it again.
type partialRangeReadError struct {
	err     error
	written int64
}

func (e *partialRangeReadError) Error() string {
	return fmt.Sprintf("dlengine: partial range read after %d bytes: %v", e.written, e.err)
}

func (e *partialRangeReadError) Unwrap() error { return e.err }
