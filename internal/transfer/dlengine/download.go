package dlengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// speedEstimator reports a short rolling-window average. Download sources can
// deliver bytes in bursts even while a request remains healthy; a rolling
// window avoids advertising a false zero-speed pause between adjacent bursts.
type speedEstimator struct {
	samples []speedSample
}

type speedSample struct {
	at         time.Time
	downloaded int64
}

func newSpeedEstimator(now time.Time, downloaded int64) *speedEstimator {
	return &speedEstimator{samples: []speedSample{{at: now, downloaded: downloaded}}}
}

func (s *speedEstimator) Observe(now time.Time, downloaded int64) int64 {
	if s == nil {
		return 0
	}
	if len(s.samples) == 0 {
		s.samples = append(s.samples, speedSample{at: now, downloaded: downloaded})
		return 0
	}
	if downloaded < s.samples[len(s.samples)-1].downloaded {
		// A failed chunk can roll back in-flight accounting. Start a fresh
		// sampling window rather than emitting a negative speed.
		s.samples = []speedSample{{at: now, downloaded: downloaded}}
		return 0
	}
	s.samples = append(s.samples, speedSample{at: now, downloaded: downloaded})

	cutoff := now.Add(-speedSampleWindow)
	baseline := 0
	for baseline+1 < len(s.samples) && !s.samples[baseline+1].at.After(cutoff) {
		baseline++
	}
	if baseline > 0 {
		s.samples = append([]speedSample(nil), s.samples[baseline:]...)
	}
	first := s.samples[0]
	elapsed := now.Sub(first.at)
	if elapsed <= 0 || downloaded <= first.downloaded {
		return 0
	}
	return int64(float64(downloaded-first.downloaded) / elapsed.Seconds())
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
			// A custom DialContext otherwise makes net/http conservatively
			// disable HTTP/2. Let CDN endpoints use HTTP/2 when available while
			// retaining HTTP/1.1 fallback for every existing provider.
			ForceAttemptHTTP2: true,
		},
	}
	defer hc.CloseIdleConnections()

	// Resolve size + range support.
	total, acceptRanges, validator, err := probe(ctx, hc, opts, url)
	if err != nil {
		return err
	}
	if total <= 0 && opts.ExpectedSize > 0 {
		total = opts.ExpectedSize
	}

	urlHash := urlFingerprint(url)
	st := &state{URLHash: urlHash, Total: total, Chunk: opts.ChunkSize, ETag: validator.ETag, LastModified: validator.LastModified}
	if total == 0 || !acceptRanges || total < opts.MinSize {
		// single stream
		return singleStream(ctx, hc, opts, url, localPath, total, validator, onProgress)
	}

	// Resume from state if it matches.
	if b, err := os.ReadFile(statePath); err == nil {
		var prev state
		if json.Unmarshal(b, &prev) == nil && stateURLMatches(prev, url, urlHash) && prev.Total == total && prev.Chunk == opts.ChunkSize &&
			resumeIdentityMatches(prev, validator) {
			st = &prev
		}
	}
	numChunks := int((total + opts.ChunkSize - 1) / opts.ChunkSize)
	if len(st.Done) != numChunks {
		st.Done = make([]bool, numChunks)
	}
	st.ETag = validator.ETag
	st.LastModified = validator.LastModified
	st.URL = ""
	st.URLHash = urlHash

	// Ensure part file exists at full size (sparse).
	if err := ensureFile(partPath, total); err != nil {
		return err
	}
	f, err := os.OpenFile(partPath, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	var limiter RateLimiter = opts.Limiter
	if limiter == nil {
		limiter = newSpeedLimiter(opts.MaxSpeed)
	}
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
			remainingStart := start
			remainingLength := length
			var lastErr error
			partialRequests := 0
			for attempt := 0; attempt < MaxRetriesPerChunk; {
				err := fetchRange(gctx, hc, opts, url, remainingStart, remainingLength, total, validator, f, limiter, func(n int64) {
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
				lastErr = err
				if errors.Is(err, errResourceChanged) || errors.Is(err, errInvalidRangeResponse) {
					if resumed := remainingStart - start; resumed > 0 {
						downloaded.Add(-resumed)
					}
					return fmt.Errorf("dlengine: chunk %d rejected: %w", chunkIdx, err)
				}
				if gctx.Err() != nil {
					if resumed := remainingStart - start; resumed > 0 {
						downloaded.Add(-resumed)
					}
					return gctx.Err()
				}
				var partial *partialRangeReadError
				if errors.As(err, &partial) && partial.written > 0 && partial.written < remainingLength {
					remainingStart += partial.written
					remainingLength -= partial.written
					partialRequests++
					if partialRequests <= MaxPartialRangeRequests {
						// This is a continuation, not a blind retry: the next request
						// starts exactly after the already persisted bytes.
						continue
					}
				}
				attempt++
				if attempt >= MaxRetriesPerChunk {
					break
				}
				select {
				case <-time.After(time.Duration(500*attempt) * time.Millisecond):
				case <-gctx.Done():
					if resumed := remainingStart - start; resumed > 0 {
						downloaded.Add(-resumed)
					}
					return gctx.Err()
				}
			}
			if resumed := remainingStart - start; resumed > 0 {
				downloaded.Add(-resumed)
			}
			return fmt.Errorf("dlengine: chunk %d failed after %d attempts: %w", chunkIdx, MaxRetriesPerChunk, lastErr)
		})
	}

	// progress ticker
	done := make(chan struct{})
	speed := newSpeedEstimator(time.Now(), downloaded.Load())
	go func() {
		ticker := time.NewTicker(progressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				cur := downloaded.Load()
				if onProgress != nil {
					onProgress(Progress{Downloaded: cur, Total: total, Speed: speed.Observe(time.Now(), cur), Percent: percent(cur, total)})
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
