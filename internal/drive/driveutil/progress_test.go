package driveutil

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProgressReaderReportsBytes(t *testing.T) {
	src := strings.NewReader("hello world")
	var reported int64
	pr := NewProgressReader(src, 11, func(read int64) {
		atomic.StoreInt64(&reported, read)
	})
	buf := make([]byte, 5)
	n, err := pr.Read(buf)
	if err != nil || n != 5 {
		t.Fatalf("Read: n=%d err=%v", n, err)
	}
	if atomic.LoadInt64(&reported) != 5 {
		t.Errorf("expected reported 5, got %d", reported)
	}
	n2, _ := pr.Read(buf)
	if atomic.LoadInt64(&reported) != int64(n+n2) {
		t.Errorf("expected reported %d, got %d", n+n2, reported)
	}
}

func TestProgressReaderThrottleNoRate(t *testing.T) {
	// When no upload rate is configured, throttle should be a no-op.
	SetUploadRateGetter(func() int64 { return 0 })
	defer SetUploadRateGetter(nil)
	src := strings.NewReader("data")
	pr := NewProgressReader(src, 4, nil)
	start := time.Now()
	io.Copy(io.Discard, pr)
	if time.Since(start) > 100*time.Millisecond {
		t.Error("throttle should not sleep when rate=0")
	}
}

func TestProgressReaderThrottleWithRate(t *testing.T) {
	// rate = 100 bytes/sec; reading 50 bytes should take ~0.5s
	SetUploadRateGetter(func() int64 { return 100 })
	defer SetUploadRateGetter(nil)
	data := strings.Repeat("x", 50)
	pr := NewProgressReader(strings.NewReader(data), 50, nil)
	start := time.Now()
	io.Copy(io.Discard, pr)
	elapsed := time.Since(start)
	// should sleep at least ~0.3s (allowing scheduling slack), at most 5s
	if elapsed < 200*time.Millisecond {
		t.Errorf("expected throttle sleep, elapsed=%v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("throttle exceeded cap: %v", elapsed)
	}
}

func TestProgressReaderNilOnRead(t *testing.T) {
	// nil onRead should not panic
	pr := NewProgressReader(strings.NewReader("test"), 4, nil)
	buf := make([]byte, 4)
	_, _ = pr.Read(buf)
}
