package drive

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
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

var uploadSessionStore UploadSessionStore

// SetUploadSessionStore installs the store-backed upload session persistence
// (called once at startup by the app wiring).
func SetUploadSessionStore(s UploadSessionStore) { uploadSessionStore = s }

// SaveUploadSession persists uploaded part numbers for a session key.
func SaveUploadSession(key string, partNumbers []int) error {
	if uploadSessionStore == nil {
		return nil
	}
	return uploadSessionStore.SaveUploadSession(key, partNumbers)
}

// LoadUploadSession returns the persisted uploaded part numbers for a key,
// or nil when no session exists.
func LoadUploadSession(key string) []int {
	if uploadSessionStore == nil {
		return nil
	}
	return uploadSessionStore.LoadUploadSession(key)
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
		if tok, err := tokenResolver(userID, driveID); err == nil && tok != nil {
			c.Token = tok
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
func ListDir(userID, driveID, dirID string, opts *ListOptions) ([]model.File, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.Search != "" {
		return d.Search(context.Background(), c, opts.Search)
	}
	return d.List(context.Background(), c, dirID, opts)
}

// ListDirPage lists one cursor page.
func ListDirPage(userID, driveID, dirID, marker string, opts *ListOptions) (*DirPage, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.ListPaged(context.Background(), c, dirID, marker, opts)
}

// SearchDir performs server-side search.
func SearchDir(userID, driveID, keyword string) ([]model.File, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Search(context.Background(), c, keyword)
}

// ListTrash lists the recycle bin.
func ListTrash(userID, driveID string, opts *ListOptions) ([]model.File, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.ListTrash(context.Background(), c, opts)
}

// GetFileInfo returns raw provider detail.
func GetFileInfo(userID, driveID, fileID string) (any, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.GetInfo(context.Background(), c, fileID)
}

// RefreshAccount refreshes an account's quota + profile from the provider and
// returns the updated token (or the original on unsupported/error). Silent +
// low-frequency caller (frontend avatar/quota popover refresh).
func RefreshAccount(userID, driveID string) (*model.TokenInfo, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	tok := c.Token
	if tok == nil {
		return nil, ErrUnknownProvider
	}
	return d.RefreshAccount(context.Background(), c, tok)
}

// GetFile returns the unified file model (from cache if present).
func GetFile(userID, driveID, fileID string) (*model.File, error) {
	if f, ok := fileCache.Get(driveID, fileID); ok {
		return &f, nil
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	f, err := d.GetFile(context.Background(), c, fileID)
	if err != nil {
		return nil, err
	}
	remember(driveID, fileID, f)
	return f, nil
}

// GetDownloadURL resolves a download source.
func GetDownloadURL(userID, driveID, fileID string, expireSec int) (*model.DownloadURL, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.GetDownloadURL(context.Background(), c, fileID, expireSec)
}

// GetVideoPreview resolves playback sources.
func GetVideoPreview(userID, driveID, fileID string) (*model.VideoPreview, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.GetVideoPreview(context.Background(), c, fileID)
}

// Mkdir creates a folder.
func Mkdir(userID, driveID, parentID, name string) (*MkdirResult, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Mkdir(context.Background(), c, parentID, name)
}

// RenameBatch renames files one-to-one with names.
func RenameBatch(userID, driveID string, fileRefs []FileRef, names []string) ([]RenameResult, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	out := make([]RenameResult, 0, len(fileRefs))
	for i, ref := range fileRefs {
		name := names[i]
		r, err := d.Rename(context.Background(), c, ref.ID, name)
		if err != nil {
			out = append(out, RenameResult{FileID: ref.ID, ParentFileID: "", Name: name})
			continue
		}
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

// TrashBatch moves files to the recycle bin.
func TrashBatch(userID, driveID string, fileIDs []string) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Trash(context.Background(), c, fileIDs)
}

// DeleteBatch permanently deletes files (skips trash where implemented).
func DeleteBatch(userID, driveID string, fileIDs []FileRef) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Delete(context.Background(), c, fileIDs)
}

// RestoreBatch restores files from the recycle bin.
func RestoreBatch(userID, driveID string, fileIDs []string) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Restore(context.Background(), c, fileIDs)
}

// MoveBatch moves files to a target folder.
func MoveBatch(userID, driveID string, fileIDs []FileRef, toParentID, toParentDesc string) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Move(context.Background(), c, fileIDs, toParentID, toParentDesc)
}

// CopyBatch copies files into a target folder.
func CopyBatch(userID, driveID string, fileIDs []FileRef, toParentID, toParentDesc string) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Copy(context.Background(), c, fileIDs, toParentID, toParentDesc)
}

// FavoriteBatch sets/clears remote favorite on files.
func FavoriteBatch(userID, driveID string, favorite bool, fileIDs []string) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.Favorite(context.Background(), c, fileIDs, favorite)
}

// CreateShare creates a share.
func CreateShare(userID, driveID string, params ShareParams) (*model.ShareItem, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.CreateShare(context.Background(), c, params)
}

// RapidUploadByHash attempts fingerprint秒传 on the target drive.
func RapidUploadByHash(userID, driveID string, req RapidUploadRequest) (*RapidUploadResult, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	return d.RapidUploadByHash(context.Background(), c, req)
}

// ResolveTransferHash computes/reads a content fingerprint.
func ResolveTransferHash(userID, driveID, fileID, method string, allowStream bool) (string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return "", err
	}
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
		return d.UploadOneFile(ctx, c, ui)
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
		return su.UploadStream(ctx, c, parentID, name, size, reader)
	}, nil
}

// ImportShare parses a share link and returns the file listing + session
// state. The session must be passed back to SaveImportedShare to transfer
// selected files. Returns ErrNotImplemented when the provider does not
// support share import.
func ImportShare(userID, driveID, shareURL, password string) (*ShareImportSession, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	sd, ok := d.(ShareImportDriver)
	if !ok {
		return nil, ErrNotImplemented
	}
	return sd.ImportShareSession(context.Background(), c, shareURL, password)
}

// SaveImportedShare transfers selected files from a parsed share session
// into the account's folder toParentID. Returns the provider-side ids of
// the saved files.
func SaveImportedShare(userID, driveID string, session *ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
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
