package transfer

import "mnemo-go/internal/store"

const maxPerDownloadConnections = 4

// DefaultConcurrencyFromSettings returns a sane default concurrency.
// Deprecated: use concurrencyFromSettings which reads the real value.
func DefaultConcurrencyFromSettings() int {
	return maxPerDownloadConnections
}

// concurrencyFromSettings derives the per-file range connection count without
// allowing the global queue setting to multiply resource use unchecked. At
// most four buffers/connections are used for one file; the manager separately
// controls how many files run at once.
func concurrencyFromSettings(s store.Settings) int {
	n := s.MaxConcurrentDownloads
	if n <= 0 {
		n = 3
	}
	if n > maxPerDownloadConnections {
		return maxPerDownloadConnections
	}
	return n
}
