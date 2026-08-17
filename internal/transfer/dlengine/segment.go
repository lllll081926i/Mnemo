// Package dlengine implements a native multi-connection segmented downloader
// (aria2-style). It replaces the external aria2c.exe dependency: files are
// fetched with parallel HTTP Range requests and assembled at offsets, with
// resume support, per-chunk retry, global speed limiting and progress events.
package dlengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mnemo-go/internal/netx"

	"golang.org/x/sync/errgroup"
)

// Defaults mirroring sane aria2 behaviour.
const (
	DefaultConcurrency = 8
	DefaultChunkSize   = 16 << 20 // 16 MiB per chunk
	DefaultMinSize     = 32 << 20 // below this: single stream
	MaxRetriesPerChunk = 5
)

// Options tunes the download behaviour.
type Options struct {
	Concurrency int   // parallel connections
	ChunkSize   int64 // range chunk size
	MinSize     int64 // files smaller than this use a single stream
	MaxSpeed    int64 // global speed cap in bytes/s (0 = unlimited)
	Headers     map[string]string
	UserAgent   string
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
	URL          string `json:"url"`
	Total        int64  `json:"total"`
	Chunk        int64  `json:"chunk"`
	Done         []bool `json:"done"`
	LastModified string `json:"last_modified,omitempty"`
}

// Download fetches url to localPath (plus a .part temp file) using segmented
// parallel range requests. It resumes from the .part file when present.
func Download(ctx context.Context, opts Options, url, localPath string, onProgress func(Progress)) error {
	opts.Normalize()
	partPath := localPath + ".part"
	statePath := localPath + ".state.json"

	// 不能用 http.Client.Timeout（它是含 body 读取的总超时，大文件必超时）。
	// 只限制连接/响应头阶段，body 读取不设总时限（对齐 aria2 的空闲超时语义）。
	hc := &http.Client{
		Transport: &http.Transport{
			Proxy: proxyFunc(),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 60 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	// Resolve size + range support.
	total, acceptRanges, lastModified, err := probe(ctx, hc, opts, url)
	if err != nil {
		return err
	}

	st := &state{URL: url, Total: total, Chunk: opts.ChunkSize}
	if total == 0 || !acceptRanges || total < opts.MinSize {
		// single stream
		return singleStream(ctx, hc, opts, url, localPath, total, onProgress)
	}

	// Resume from state if it matches.
	if b, err := os.ReadFile(statePath); err == nil {
		var prev state
		if json.Unmarshal(b, &prev) == nil && prev.URL == url && prev.Total == total && prev.Chunk == opts.ChunkSize &&
			(prev.LastModified == "" || lastModified == "" || prev.LastModified == lastModified) {
			st = &prev
		}
	}
	numChunks := int((total + opts.ChunkSize - 1) / opts.ChunkSize)
	if len(st.Done) != numChunks {
		st.Done = make([]bool, numChunks)
	}
	st.LastModified = lastModified

	// Ensure part file exists at full size (sparse).
	if err := ensureFile(partPath, total); err != nil {
		return err
	}
	f, err := os.OpenFile(partPath, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	limiter := newSpeedLimiter(opts.MaxSpeed)
	var downloaded atomic.Int64
	// account already-done bytes
	for i, done := range st.Done {
		if done {
			downloaded.Add(chunkLen(st, i))
		}
	}

	sem := make(chan struct{}, opts.Concurrency)
	g, gctx := errgroup.WithContext(ctx)
	var mu sync.Mutex // guards st.Done writes

	for i := 0; i < numChunks; i++ {
		if st.Done[i] {
			continue
		}
		chunkIdx := i
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-gctx.Done():
				return gctx.Err()
			}
			start := int64(chunkIdx) * opts.ChunkSize
			length := chunkLen(st, chunkIdx)
			for attempt := 0; attempt < MaxRetriesPerChunk; attempt++ {
				err := fetchRange(gctx, hc, opts, url, start, length, f, limiter, func(n int64) {
					downloaded.Add(n)
				})
				if err == nil {
					mu.Lock()
					st.Done[chunkIdx] = true
					// Persist while holding the same lock that protects st.Done;
					// otherwise parallel chunks can overwrite each other's state.
					_ = persistState(statePath, st)
					mu.Unlock()
					return nil
				}
				if gctx.Err() != nil {
					return gctx.Err()
				}
				select {
				case <-time.After(time.Duration(500*(attempt+1)) * time.Millisecond):
				case <-gctx.Done():
					return gctx.Err()
				}
			}
			return fmt.Errorf("dlengine: chunk %d failed", chunkIdx)
		})
	}

	// progress ticker
	done := make(chan struct{})
	var lastBytes int64
	var speed int64
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				cur := downloaded.Load()
				speed = (cur - lastBytes) * 2
				lastBytes = cur
				if onProgress != nil {
					onProgress(Progress{Downloaded: cur, Total: total, Speed: speed, Percent: percent(cur, total)})
				}
			}
		}
	}()

	err = g.Wait()
	close(done)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	_ = f.Close()

	// sanity check
	if downloaded.Load() != total {
		return fmt.Errorf("dlengine: downloaded %d != total %d", downloaded.Load(), total)
	}
	// commit: rename part -> final
	if err := commitPart(partPath, localPath); err != nil {
		return err
	}
	_ = os.Remove(statePath)
	if onProgress != nil {
		onProgress(Progress{Downloaded: total, Total: total, Speed: 0, Percent: 100})
	}
	return nil
}

// probe determines the file size, range support and last-modified.
func probe(ctx context.Context, hc *http.Client, opts Options, url string) (int64, bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, "", err
	}
	setHeaders(req, opts.Headers, opts.UserAgent)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := hc.Do(req)
	if err != nil {
		return 0, false, "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	// A server advertising Accept-Ranges can still ignore a particular probe.
	// Only a real 206 response proves that segmented requests are usable.
	acceptRanges := resp.StatusCode == http.StatusPartialContent
	lastModified := resp.Header.Get("Last-Modified")
	var total int64
	switch {
	case resp.StatusCode == http.StatusPartialContent:
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			total = parseContentRangeTotal(cr)
		}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		total = resp.ContentLength
	default:
		return 0, false, "", fmt.Errorf("dlengine: probe http %d", resp.StatusCode)
	}
	if total <= 0 {
		return 0, false, "", errors.New("dlengine: unknown file size")
	}
	return total, acceptRanges, lastModified, nil
}

func parseContentRangeTotal(cr string) int64 {
	// "bytes 0-1/12345"
	i := strings.LastIndex(cr, "/")
	if i < 0 {
		return 0
	}
	total, _ := strconv.ParseInt(cr[i+1:], 10, 64)
	return total
}

// fetchRange GETs one range and writes it at the file offset.
func fetchRange(ctx context.Context, hc *http.Client, opts Options, url string, start, length int64, f *os.File, limiter *speedLimiter, account func(int64)) error {
	if length <= 0 {
		return errors.New("dlengine: invalid range length")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, opts.Headers, opts.UserAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+length-1))
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dlengine: range http %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		return errors.New("dlengine: server ignored range request")
	}
	buf := make([]byte, 256<<10)
	written := int64(0)
	committed := false
	defer func() {
		if !committed && written > 0 && account != nil {
			// A retry overwrites the same range. Roll back bytes reported by a
			// failed attempt so progress reflects committed content only.
			account(-written)
		}
	}()
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			limiter.waitN(int64(n))
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
			return err
		}
	}
	if written != length {
		return fmt.Errorf("dlengine: short read %d != %d", written, length)
	}
	committed = true
	return nil
}

func singleStream(ctx context.Context, hc *http.Client, opts Options, url, localPath string, total int64, onProgress func(Progress)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, opts.Headers, opts.UserAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dlengine: http %d", resp.StatusCode)
	}
	partPath := localPath + ".part"
	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	limiter := newSpeedLimiter(opts.MaxSpeed)
	buf := make([]byte, 256<<10)
	var written int64
	last := time.Now()
	var lastBytes int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			limiter.waitN(int64(n))
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if onProgress != nil && time.Since(last) >= 500*time.Millisecond {
				speed := (written - lastBytes) * 1000 / int64(time.Since(last)/time.Millisecond)
				lastBytes = written
				last = time.Now()
				onProgress(Progress{Downloaded: written, Total: total, Speed: speed, Percent: percent(written, total)})
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

// commitPart replaces the destination on every platform. Windows refuses to
// rename over an existing file, so move the old destination aside first and
// restore it if the commit fails.
func commitPart(partPath, localPath string) error {
	if err := os.Rename(partPath, localPath); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	if _, err := os.Stat(localPath); err != nil {
		return err
	}
	backup, err := os.CreateTemp(filepath.Dir(localPath), ".mnemo-replace-*")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if err := os.Rename(localPath, backupPath); err != nil {
		return err
	}
	if err := os.Rename(partPath, localPath); err != nil {
		_ = os.Rename(backupPath, localPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func setHeaders(req *http.Request, headers map[string]string, ua string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
}

func ensureFile(path string, size int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if size < 0 {
		return errors.New("dlengine: negative file size")
	}
	cur, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if cur != size {
		if err := f.Truncate(size); err != nil {
			return err
		}
	}
	if cur < size {
		if _, err := f.Seek(size-1, io.SeekStart); err != nil {
			return err
		}
		if _, err := f.Write([]byte{0}); err != nil {
			return err
		}
	}
	return nil
}

func chunkLen(st *state, idx int) int64 {
	start := int64(idx) * st.Chunk
	if start+st.Chunk > st.Total {
		return st.Total - start
	}
	return st.Chunk
}

func percent(done, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(done * 100 / total)
}

func persistState(path string, st *state) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// speedLimiter is a token-bucket style throttle (per second).
type speedLimiter struct {
	mu     sync.Mutex
	rate   int64
	window int64
	start  time.Time
}

func newSpeedLimiter(rate int64) *speedLimiter {
	return &speedLimiter{rate: rate, start: time.Now()}
}

func (l *speedLimiter) waitN(n int64) {
	if l.rate <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// slide the window: if more than 1s has passed since the last reset,
	// reset the accumulator and the start timestamp so the limiter keeps
	// tracking the recent rate rather than a lifetime average that drifts.
	if now.Sub(l.start) >= time.Second {
		l.start = now
		l.window = 0
	}
	l.window += n
	elapsed := now.Sub(l.start).Seconds()
	allowed := float64(l.rate) * elapsed
	if float64(l.window) > allowed {
		wait := time.Duration((float64(l.window)-allowed)/float64(l.rate)*1000) * time.Millisecond
		if wait > 0 && wait < 10*time.Second {
			l.mu.Unlock()
			time.Sleep(wait)
			l.mu.Lock()
		}
	}
}

var _ = filepath.Base

// proxyFunc returns a proxy function that honors the netx global proxy first,
// falling back to the environment (HTTP_PROXY/HTTPS_PROXY).
func proxyFunc() func(*http.Request) (*url.URL, error) {
	gp := netx.GlobalProxy()
	if gp != "" {
		if u, err := url.Parse(gp); err == nil && u.Scheme != "" {
			return http.ProxyURL(u)
		}
	}
	return http.ProxyFromEnvironment
}
