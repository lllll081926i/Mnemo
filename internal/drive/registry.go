package drive

import (
	"sort"
)

// Registration is a provider plugin registration. Providers construct a
// Factory that builds a fresh Driver per account instance.
type Registration struct {
	ID      string
	Meta    Meta
	Caps    Capabilities
	Factory func() Driver
}

var registry = map[string]Registration{}

// Register installs a provider plugin. Called from each provider package's
// init(). Panics on duplicate or empty ids.
func Register(r Registration) {
	if r.ID == "" {
		panic("drive.Register: empty provider id")
	}
	if _, dup := registry[r.ID]; dup {
		panic("drive.Register: duplicate provider " + r.ID)
	}
	registry[r.ID] = r
}

// Get returns the registration of a provider.
func Get(provider string) (Registration, bool) {
	r, ok := registry[provider]
	return r, ok
}

// Require returns the registration or panics with a helpful message.
func Require(provider string) Registration {
	r, ok := registry[provider]
	if !ok {
		panic("drive.Require: provider " + provider + " is not registered")
	}
	return r
}

// IsRegistered reports whether a provider plugin exists.
func IsRegistered(provider string) bool {
	_, ok := registry[provider]
	return ok
}

// All returns every registration sorted by id.
func All() []Registration {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Registration, 0, len(keys))
	for _, k := range keys {
		out = append(out, registry[k])
	}
	return out
}

// Count returns the number of registered plugins.
func Count() int { return len(registry) }

// New builds a fresh Driver instance for a provider.
func New(provider string) Driver {
	return Require(provider).Factory()
}