package transfer

import "mnemo-go/internal/store"

// DefaultConcurrencyFromSettings returns a sane default concurrency.
// Deprecated: use concurrencyFromSettings which reads the real value.
func DefaultConcurrencyFromSettings() int {
	return 8
}

// concurrencyFromSettings reads MaxConcurrentDownloads from settings with a
// sane fallback. The per-file connection count (DefaultConcurrency = 8) is
// applied on top of this semaphore.
func concurrencyFromSettings(s store.Settings) int {
	if s.MaxConcurrentDownloads > 0 {
		return s.MaxConcurrentDownloads
	}
	return 3
}
