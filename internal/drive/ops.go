package drive

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

// ListDir lists a directory (search when opts.Search is set).
func ListDir(userID, driveID, dirID string, opts *ListOptions) (files []model.File, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	if opts != nil && opts.Search != "" {
		return d.Search(context.Background(), c, opts.Search)
	}
	return d.List(context.Background(), c, dirID, opts)
}

// ListDirPage lists one cursor page.
func ListDirPage(userID, driveID, dirID, marker string, opts *ListOptions) (page *DirPage, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.ListPaged(context.Background(), c, dirID, marker, opts)
}

// ListDirAll consumes cursor pages when a provider exposes pagination. The
// guard prevents a broken provider cursor from hanging a migration forever.
func ListDirAll(userID, driveID, dirID string, opts *ListOptions) ([]model.File, error) {
	var out []model.File
	marker := ""
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		p, err := ListDirPage(userID, driveID, dirID, marker, opts)
		if err != nil {
			if page == 0 {
				return ListDir(userID, driveID, dirID, opts)
			}
			return nil, err
		}
		if p == nil {
			return out, nil
		}
		out = append(out, p.Items...)
		if p.NextMarker == "" {
			return out, nil
		}
		if seen[p.NextMarker] {
			return nil, errors.New("drive: duplicate list cursor")
		}
		seen[p.NextMarker] = true
		marker = p.NextMarker
	}
	return nil, errors.New("drive: list pagination exceeded limit")
}

// SearchDir performs server-side search.
func SearchDir(userID, driveID, keyword string) (files []model.File, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Search(context.Background(), c, keyword)
}

// ListTrash lists the recycle bin.
func ListTrash(userID, driveID string, opts *ListOptions) (files []model.File, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.ListTrash(context.Background(), c, opts)
}

// GetFileInfo returns raw provider detail.
func GetFileInfo(userID, driveID, fileID string) (info any, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.GetInfo(context.Background(), c, fileID)
}

// RefreshAccount refreshes an account's quota + profile from the provider and
// returns the updated token (or the original on unsupported/error). Silent +
// low-frequency caller (frontend avatar/quota popover refresh).
func RefreshAccount(userID, driveID string) (token *model.TokenInfo, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	tok := c.Token
	if tok == nil {
		return nil, ErrUnknownProvider
	}
	token, err = d.RefreshAccount(context.Background(), c, tok)
	if token != nil && token != c.Token {
		c.Token = token
	}
	err = withTokenPersist(err, c)
	return token, err
}

// ValidateConnection lets mounted-storage providers verify a connection
// before the app persists it as an account.
func ValidateConnection(provider string, conn *model.ConnConfig) error {
	d, ok := Get(provider)
	if !ok {
		return ErrUnknownProvider
	}
	validator, ok := d.Factory().(ConnectionValidator)
	if !ok {
		return NotSupported("validateConnection")
	}
	return validator.ValidateConnection(context.Background(), conn)
}

// GetFile returns the unified file model (from cache if present).
func GetFile(userID, driveID, fileID string) (file *model.File, err error) {
	if f, ok := fileCache.Get(userID, driveID, fileID); ok {
		return &f, nil
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	file, err = d.GetFile(context.Background(), c, fileID)
	if err != nil {
		return nil, err
	}
	remember(userID, driveID, fileID, file)
	return file, nil
}

// GetDownloadURL resolves a download source.
func GetDownloadURL(userID, driveID, fileID string, expireSec int) (url *model.DownloadURL, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.GetDownloadURL(context.Background(), c, fileID, expireSec)
}

// GetVideoPreview resolves playback sources.
func GetVideoPreview(userID, driveID, fileID string) (preview *model.VideoPreview, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.GetVideoPreview(context.Background(), c, fileID)
}

// Mkdir creates a folder.
func Mkdir(userID, driveID, parentID, name string) (result *MkdirResult, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Mkdir(context.Background(), c, parentID, name)
}

// RenameBatch renames files one-to-one with names.
func RenameBatch(userID, driveID string, fileRefs []FileRef, names []string) (out []RenameResult, err error) {
	if len(fileRefs) != len(names) {
		return nil, errors.New("drive: rename refs and names length mismatch")
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	out = make([]RenameResult, 0, len(fileRefs))
	var renameErrs []error
	for i, ref := range fileRefs {
		name := names[i]
		r, err := d.Rename(context.Background(), c, ref.ID, name)
		if err != nil {
			out = append(out, RenameResult{FileID: "", ParentFileID: "", Name: name})
			renameErrs = append(renameErrs, fmt.Errorf("file %s: %w", ref.ID, err))
			continue
		}
		if r != nil {
			out = append(out, *r)
			continue
		}
		out = append(out, RenameResult{FileID: "", ParentFileID: "", Name: name})
		renameErrs = append(renameErrs, fmt.Errorf("file %s: provider returned an empty rename result", ref.ID))
	}
	return out, errors.Join(renameErrs...)
}

// TrashBatch moves files to the recycle bin.
func TrashBatch(userID, driveID string, fileIDs []string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Trash(context.Background(), c, fileIDs)
}

// DeleteBatch permanently deletes files (skips trash where implemented).
func DeleteBatch(userID, driveID string, fileIDs []FileRef) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Delete(context.Background(), c, fileIDs)
}

// RestoreBatch restores files from the recycle bin.
func RestoreBatch(userID, driveID string, fileIDs []string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Restore(context.Background(), c, fileIDs)
}

// MoveBatch moves files to a target folder.
func MoveBatch(userID, driveID string, fileIDs []FileRef, toParentID, toParentDesc string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Move(context.Background(), c, fileIDs, toParentID, toParentDesc)
}

// CopyBatch copies files into a target folder.
func CopyBatch(userID, driveID string, fileIDs []FileRef, toParentID, toParentDesc string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Copy(context.Background(), c, fileIDs, toParentID, toParentDesc)
}

// FavoriteBatch sets/clears remote favorite on files.
func FavoriteBatch(userID, driveID string, favorite bool, fileIDs []string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Favorite(context.Background(), c, fileIDs, favorite)
}

// CreateShare creates a share.
func CreateShare(userID, driveID string, params ShareParams) (share *model.ShareItem, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.CreateShare(context.Background(), c, params)
}

// RapidUploadByHash attempts fingerprint秒传 on the target drive.
func RapidUploadByHash(userID, driveID string, req RapidUploadRequest) (result *RapidUploadResult, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.RapidUploadByHash(context.Background(), c, req)
}

// ResolveTransferHash computes/reads a content fingerprint.
func ResolveTransferHash(userID, driveID, fileID, method string, allowStream bool) (hash string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return "", err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.ResolveTransferHash(context.Background(), c, fileID, method, allowStream)
}

// QueueUploadHandler returns the driver's single-file upload handler for an
// account, or nil when the provider exposes none.
func QueueUploadHandler(userID, driveID string) (func(ctx context.Context, ui *model.UploadingUI) error, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, ui *model.UploadingUI) error {
		opErr := d.UploadOneFile(ctx, c, ui)
		return withTokenPersist(opErr, c)
	}, nil
}

// StreamUploadHandler returns a function that streams an upload from an
// io.Reader into the target provider, or (nil, ErrNotImplemented) when the
// provider does not implement the StreamUploader capability.
func StreamUploadHandler(userID, driveID string) (func(ctx context.Context, parentID, name string, size int64, reader io.Reader) error, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	su, ok := d.(StreamUploader)
	if !ok {
		return nil, ErrNotImplemented
	}
	return func(ctx context.Context, parentID, name string, size int64, reader io.Reader) error {
		opErr := su.UploadStream(ctx, c, parentID, name, size, reader)
		return withTokenPersist(opErr, c)
	}, nil
}

// ImportShare parses a share link and returns the file listing + session
// state. The session must be passed back to SaveImportedShare to transfer
// selected files. Returns ErrNotImplemented when the provider does not
// support share import.
func ImportShare(userID, driveID, shareURL, password string) (session *ShareImportSession, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	sd, ok := d.(ShareImportDriver)
	if !ok {
		return nil, ErrNotImplemented
	}
	return sd.ImportShareSession(context.Background(), c, shareURL, password)
}

// SaveImportedShare transfers selected files from a parsed share session
// into the account's folder toParentID. Returns the provider-side ids of
// the saved files.
func SaveImportedShare(userID, driveID string, session *ShareImportSession, fileIDs []string, toParentID string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	sd, ok := d.(ShareImportDriver)
	if !ok {
		return nil, ErrNotImplemented
	}
	return sd.SaveShare(context.Background(), c, session, fileIDs, toParentID)
}

// RootID returns the provider root id for an account.
func RootID(userID, driveID string) (string, error) {
	provider := ResolveProvider(userID, driveID, "")
	if m := GetMeta(provider); m.RootKey != "" {
		return m.RootKey, nil
	}
	return provider + "_root", nil
}

// ProviderOf returns the resolved provider for an account context.
func ProviderOf(userID, driveID, tokenFrom string) string {
	return ResolveProvider(userID, driveID, tokenFrom)
}
