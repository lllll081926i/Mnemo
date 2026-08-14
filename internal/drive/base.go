package drive

import (
	"context"

	"mnemo-go/internal/model"
)

// BaseDriver provides default no-op/error implementations for the optional
// Driver methods. Providers embed BaseDriver and override only the capabilities
// they implement. Required, always-implemented methods remain on the concrete
// type (List is the minimum contract).
type BaseDriver struct{}

func (BaseDriver) ID() string { return "" }
func (BaseDriver) Meta() Meta { return Meta{} }
func (BaseDriver) Capabilities() Capabilities { return Capabilities{} }
func (BaseDriver) RootID() string { return "root" }

func (BaseDriver) ListPaged(ctx context.Context, c Context, dirID, marker string, opts *ListOptions) (*DirPage, error) {
	return nil, NotSupported("listPaged")
}
func (BaseDriver) ListTrash(ctx context.Context, c Context, opts *ListOptions) ([]model.File, error) {
	return nil, NotSupported("listTrash")
}
func (BaseDriver) Search(ctx context.Context, c Context, keyword string) ([]model.File, error) {
	return nil, NotSupported("search")
}
func (BaseDriver) GetInfo(ctx context.Context, c Context, fileID string) (any, error) {
	return nil, NotSupported("getInfo")
}
func (BaseDriver) GetFile(ctx context.Context, c Context, fileID string) (*model.File, error) {
	return nil, NotSupported("getFile")
}
func (BaseDriver) GetDownloadURL(ctx context.Context, c Context, fileID string, expireSec int) (*model.DownloadURL, error) {
	return nil, NotSupported("getDownloadUrl")
}
func (BaseDriver) GetVideoPreview(ctx context.Context, c Context, fileID string) (*model.VideoPreview, error) {
	return nil, NotSupported("getVideoPreview")
}
func (BaseDriver) Trash(ctx context.Context, c Context, fileIDs []string) ([]string, error) {
	return nil, NotSupported("trash")
}
func (BaseDriver) Restore(ctx context.Context, c Context, fileIDs []string) ([]string, error) {
	return nil, NotSupported("restore")
}
func (BaseDriver) Favorite(ctx context.Context, c Context, fileIDs []string, favorite bool) ([]string, error) {
	return nil, NotSupported("favorite")
}
func (BaseDriver) CreateShare(ctx context.Context, c Context, params ShareParams) (*model.ShareItem, error) {
	return nil, NotSupported("createShare")
}
func (BaseDriver) UploadOneFile(ctx context.Context, c Context, ui *model.UploadingUI) error {
	return NotSupported("upload")
}
func (BaseDriver) RapidUploadByHash(ctx context.Context, c Context, req RapidUploadRequest) (*RapidUploadResult, error) {
	return nil, NotSupported("rapidUploadByHash")
}
func (BaseDriver) ResolveTransferHash(ctx context.Context, c Context, fileID, method string, allowStream bool) (string, error) {
	return "", NotSupported("resolveTransferHash")
}
func (BaseDriver) RefreshAccount(ctx context.Context, c Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	return nil, nil
}