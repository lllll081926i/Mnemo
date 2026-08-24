package drive

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"mnemo-go/internal/model"
)

// TokenResolver resolves the stored session for an account. It is wired by the
// app layer (which owns the store) to keep drive agnostic of persistence.
type TokenResolver func(userID, driveID string) (*model.TokenInfo, error)

var tokenResolver TokenResolver

// SetTokenResolver installs the store-backed token resolver (called once at
// startup by the app wiring).
func SetTokenResolver(fn TokenResolver) { tokenResolver = fn }

// TokenUpdater persists a provider session after an operation transparently
// refreshes or re-logins. The app layer owns the backing store.
type TokenUpdater func(userID, driveID string, token *model.TokenInfo) error

var tokenUpdater TokenUpdater

func SetTokenUpdater(fn TokenUpdater) { tokenUpdater = fn }

// CloneToken prevents concurrent provider calls from mutating the store's
// in-memory account object before the refreshed session is persisted.
func CloneToken(tok *model.TokenInfo) *model.TokenInfo {
	if tok == nil {
		return nil
	}
	out := *tok
	if tok.Raw != nil {
		out.Raw = append([]byte(nil), tok.Raw...)
	}
	if tok.Conn != nil {
		conn := *tok.Conn
		out.Conn = &conn
	}
	return &out
}

func persistToken(c Context) error {
	if tokenUpdater == nil || c.Token == nil {
		return nil
	}
	if c.TokenSnapshot != nil && reflect.DeepEqual(c.TokenSnapshot, c.Token) {
		return nil
	}
	return tokenUpdater(c.UserID, c.DriveID, CloneToken(c.Token))
}

func withTokenPersist(opErr error, c Context) error {
	return errors.Join(opErr, persistToken(c))
}

// SecretResolver returns the OAuth client credentials for a provider by key
// (e.g. "onedrive_client_id", "dropbox_app_key"). It is wired by the app
// layer so providers can read secrets during RefreshAccount without depending
// on the config package.
type SecretResolver func(key string) string

var secretResolver SecretResolver

// SetSecretResolver installs the app-backed secret resolver (called once at
// startup by the app wiring).
func SetSecretResolver(fn SecretResolver) { secretResolver = fn }

// Secret returns the configured value for key, or "" when unset.
func Secret(key string) string {
	if secretResolver == nil {
		return ""
	}
	return secretResolver(key)
}

// ---- Upload session persistence (resumable uploads) ----

// UploadSessionStore abstracts per-key upload session persistence so the
// drive package stays free of store imports. The app layer wires it at
// startup via SetUploadSessionStore.
type UploadSessionStore interface {
	SaveUploadSession(key string, partNumbers []int) error
	LoadUploadSession(key string) []int
	ClearUploadSession(key string)
}

type uploadSessionStateStore interface {
	SaveUploadSessionState(key, sessionID string, partNumbers []int) error
	LoadUploadSessionState(key string) (string, []int)
}

var uploadSessionStore UploadSessionStore
var uploadSessionState uploadSessionStateStore

// SetUploadSessionStore installs the store-backed upload session persistence
// (called once at startup by the app wiring).
func SetUploadSessionStore(s UploadSessionStore) {
	uploadSessionStore = s
	if state, ok := s.(uploadSessionStateStore); ok {
		uploadSessionState = state
	} else {
		uploadSessionState = nil
	}
}

// SaveUploadSession persists uploaded part numbers for a session key.
func SaveUploadSession(key string, partNumbers []int) error {
	if uploadSessionStore == nil {
		return nil
	}
	return uploadSessionStore.SaveUploadSession(key, partNumbers)
}

// SaveUploadSessionState persists a provider session id with completed parts.
func SaveUploadSessionState(key, sessionID string, partNumbers []int) error {
	if uploadSessionState != nil {
		return uploadSessionState.SaveUploadSessionState(key, sessionID, partNumbers)
	}
	return SaveUploadSession(key, partNumbers)
}

// LoadUploadSession returns the persisted uploaded part numbers for a key,
// or nil when no session exists.
func LoadUploadSession(key string) []int {
	if uploadSessionStore == nil {
		return nil
	}
	return uploadSessionStore.LoadUploadSession(key)
}

// LoadUploadSessionState loads a provider session id and completed parts.
func LoadUploadSessionState(key string) (string, []int) {
	if uploadSessionState != nil {
		return uploadSessionState.LoadUploadSessionState(key)
	}
	return "", LoadUploadSession(key)
}

// ClearUploadSession removes the persisted session for a key.
func ClearUploadSession(key string) {
	if uploadSessionStore == nil {
		return
	}
	uploadSessionStore.ClearUploadSession(key)
}

// UploadSessionKey computes a stable hash key from the tuple
// userID:driveID:parentID:name:size, suitable for deduplicating resume state.
func UploadSessionKey(userID, driveID, parentID, name string, size int64) string {
	raw := userID + ":" + driveID + ":" + parentID + ":" + name + ":" + formatSize(size)
	h := sha1.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

func formatSize(n int64) string { return strconv.FormatInt(n, 10) }

// SortedUniqueParts deduplicates and sorts part numbers for stable persistence.
func SortedUniqueParts(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// BuildContext resolves the provider + session for an account.
func BuildContext(userID, driveID, tokenFrom string) (Context, error) {
	provider := ResolveProvider(userID, driveID, tokenFrom)
	if provider == model.ProviderUnknown {
		return Context{}, ErrUnknownProvider
	}
	c := Context{UserID: userID, DriveID: driveID, TokenFrom: provider}
	if tokenResolver != nil {
		tok, err := tokenResolver(userID, driveID)
		if err != nil {
			return Context{}, fmt.Errorf("drive: load account session: %w", err)
		}
		if tok != nil {
			c.Token = CloneToken(tok)
			c.TokenSnapshot = CloneToken(tok)
			if c.TokenFrom == "" || c.TokenFrom == model.ProviderUnknown {
				c.TokenFrom = tok.TokenFrom
			}
		}
	}
	return c, nil
}

// DriverFor returns a configured driver instance for an account context.
func DriverFor(c Context) (Driver, error) {
	reg, ok := Get(c.TokenFrom)
	if !ok {
		// fall back to user id prefix resolution if tokenfrom not filled
		provider := ResolveProvider(c.UserID, c.DriveID, c.TokenFrom)
		reg, ok = Get(provider)
		if !ok {
			return nil, ErrUnknownProvider
		}
		c.TokenFrom = provider
	}
	return reg.Factory(), nil
}

func driverAndCtx(userID, driveID string) (Driver, Context, error) {
	c, err := BuildContext(userID, driveID, "")
	if err != nil {
		return nil, c, err
	}
	d, err := DriverFor(c)
	if err != nil {
		return nil, c, err
	}
	return d, c, nil
}
