package transfer

import "mnemo-go/internal/transfer/dlengine"

// DefaultConcurrencyFromSettings returns a sane default concurrency.
func DefaultConcurrencyFromSettings() int {
	return dlengine.DefaultConcurrency
}
