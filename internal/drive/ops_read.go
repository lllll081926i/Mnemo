package drive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mnemo-go/internal/model"
)

// ListDir lists a directory (search when opts.Search is set).
func ListDir(userID, driveID, dirID string, opts *ListOptions) (files []model.File, err error) {
	return ListDirContext(context.Background(), userID, driveID, dirID, opts)
}

// ListDirContext is the cancellation-aware variant used by long-running
// workers. The Wails-compatible ListDir wrapper remains for existing callers.
func ListDirContext(ctx context.Context, userID, driveID, dirID string, opts *ListOptions) (files []model.File, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	if opts != nil && opts.Search != "" {
		return d.Search(ctx, c, opts.Search)
	}
	return d.List(ctx, c, dirID, opts)
}

// ListDirPage lists one cursor page.
func ListDirPage(userID, driveID, dirID, marker string, opts *ListOptions) (page *DirPage, err error) {
	return ListDirPageContext(context.Background(), userID, driveID, dirID, marker, opts)
}

// ListDirPageContext is the cancellation-aware cursor-page variant.
func ListDirPageContext(ctx context.Context, userID, driveID, dirID, marker string, opts *ListOptions) (page *DirPage, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.ListPaged(ctx, c, dirID, marker, opts)
}

// ListDirAll consumes cursor pages when a provider exposes pagination. The
// guard prevents a broken provider cursor from hanging a migration forever.
func ListDirAll(userID, driveID, dirID string, opts *ListOptions) ([]model.File, error) {
	return ListDirAllContext(context.Background(), userID, driveID, dirID, opts)
}

// ListDirAllContext consumes cursor pages while honoring cancellation.
func ListDirAllContext(ctx context.Context, userID, driveID, dirID string, opts *ListOptions) ([]model.File, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out []model.File
	marker := ""
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p, err := ListDirPageContext(ctx, userID, driveID, dirID, marker, opts)
		if err != nil {
			if page == 0 {
				return ListDirContext(ctx, userID, driveID, dirID, opts)
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

// ValidateUploadItems applies an optional provider upload policy before local
// files are placed in the asynchronous queue. Providers without a policy are
// intentionally a no-op.
func ValidateUploadItems(userID, driveID string, items []UploadValidationItem) error {
	return ValidateUploadItemsContext(context.Background(), userID, driveID, items)
}

// ValidateUploadItemsContext is the cancellation-aware variant used by
// cross-drive migration once it has the real local spool size. It keeps target
// constraints (for example file-type or size rules) from triggering a remote
// upload request after a source download has completed.
func ValidateUploadItemsContext(ctx context.Context, userID, driveID string, items []UploadValidationItem) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return err
	}
	defer func() { err = withTokenPersist(err, c) }()
	validator, ok := d.(UploadValidator)
	if !ok {
		return nil
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return errors.New("drive: 上传文件名为空")
		}
		if item.Size < 0 {
			return fmt.Errorf("drive: 上传文件大小无效: %s", name)
		}
		if validationErr := validator.ValidateUpload(ctx, c, name, item.Size); validationErr != nil {
			return validationErr
		}
	}
	return nil
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

// ValidateWriteConnection performs an explicitly requested mounted-storage
// write check. Providers must use an isolated temporary object and clean it up
// before returning; this is intentionally separate from login validation.
func ValidateWriteConnection(provider string, conn *model.ConnConfig) error {
	d, ok := Get(provider)
	if !ok {
		return ErrUnknownProvider
	}
	validator, ok := d.Factory().(WriteConnectionValidator)
	if !ok {
		return NotSupported("validateWriteConnection")
	}
	return validator.ValidateWriteConnection(context.Background(), conn)
}

// GetFile returns the unified file model (from cache if present).
func GetFile(userID, driveID, fileID string) (file *model.File, err error) {
	return GetFileContext(context.Background(), userID, driveID, fileID)
}

// GetFileContext resolves a file while honoring cancellation.
func GetFileContext(ctx context.Context, userID, driveID, fileID string) (file *model.File, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f, ok := fileCache.Get(userID, driveID, fileID); ok {
		return &f, nil
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	file, err = d.GetFile(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	remember(userID, driveID, fileID, file)
	return file, nil
}

// GetDownloadURL resolves a download source.
func GetDownloadURL(userID, driveID, fileID string, expireSec int) (url *model.DownloadURL, err error) {
	return GetDownloadURLContext(context.Background(), userID, driveID, fileID, expireSec)
}

// GetDownloadURLContext is the cancellation-aware download URL resolver.
func GetDownloadURLContext(ctx context.Context, userID, driveID, fileID string, expireSec int) (url *model.DownloadURL, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.GetDownloadURL(ctx, c, fileID, expireSec)
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
