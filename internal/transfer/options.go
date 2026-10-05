package transfer

import (
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer/dlengine"
)

const maxPerDownloadConnections = 16

// DefaultConcurrencyFromSettings returns a sane default concurrency.
// Deprecated: use concurrencyFromSettings which reads the real value.
func DefaultConcurrencyFromSettings() int {
	return dlengine.DefaultConcurrency
}

func concurrencyFromSettings(s store.Settings) int {
	n := s.MaxDownloadConnections
	if n <= 0 {
		n = dlengine.DefaultConcurrency
	}
	if n > maxPerDownloadConnections {
		return maxPerDownloadConnections
	}
	return n
}

func applyDownloadPolicy(opts *dlengine.Options, settings store.Settings, source *model.DownloadURL) {
	opts.Concurrency = concurrencyFromSettings(settings)
	if source.Concurrency > 0 && source.Concurrency < opts.Concurrency {
		opts.Concurrency = source.Concurrency
	}
	opts.ChunkSize = source.ChunkSize
	opts.Headers = source.Headers
	opts.RequestAuth = source.RequestAuth
	opts.RetrySingleConnection = source.RetrySingleConnection
}

func (m *Manager) DownloadOptions() (dlengine.Options, error) {
	settings, err := m.store.GetSettings()
	if err != nil {
		return dlengine.Options{}, err
	}
	m.speedLimiter.SetRate(settings.MaxDownloadSpeed)
	return dlengine.Options{Concurrency: concurrencyFromSettings(settings), Limiter: m.speedLimiter}, nil
}

func (m *Manager) SetDownloadSpeed(rate int64) {
	m.speedLimiter.SetRate(rate)
}
