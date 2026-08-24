package drive

import (
	"context"
	"io"

	"mnemo-go/internal/model"
)

// RapidUploadByHash attempts fingerprint秒传 on the target drive.
func RapidUploadByHash(userID, driveID string, req RapidUploadRequest) (result *RapidUploadResult, err error) {
	return RapidUploadByHashContext(context.Background(), userID, driveID, req)
}

// RapidUploadByHashContext attempts fingerprint upload while honoring cancellation.
func RapidUploadByHashContext(ctx context.Context, userID, driveID string, req RapidUploadRequest) (result *RapidUploadResult, err error) {
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
	return d.RapidUploadByHash(ctx, c, req)
}

// ResolveTransferHash computes/reads a content fingerprint.
func ResolveTransferHash(userID, driveID, fileID, method string, allowStream bool) (hash string, err error) {
	return ResolveTransferHashContext(context.Background(), userID, driveID, fileID, method, allowStream)
}

// ResolveTransferHashContext computes a transfer fingerprint while honoring cancellation.
func ResolveTransferHashContext(ctx context.Context, userID, driveID, fileID, method string, allowStream bool) (hash string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return "", err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.ResolveTransferHash(ctx, c, fileID, method, allowStream)
}

// QueueUploadHandler returns the driver's single-file upload handler for an
// account, or nil when the provider exposes none.
func QueueUploadHandler(userID, driveID string) (func(ctx context.Context, ui *model.UploadingUI) error, error) {
	return QueueUploadHandlerContext(context.Background(), userID, driveID)
}

// QueueUploadHandlerContext resolves a provider handler while honoring the
// caller's setup context. The returned handler continues to receive its own
// operation context as before.
func QueueUploadHandlerContext(ctx context.Context, userID, driveID string) (func(context.Context, *model.UploadingUI) error, error) {
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
	if err := ctx.Err(); err != nil {
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
	return StreamUploadHandlerContext(context.Background(), userID, driveID)
}

// StreamUploadHandlerContext resolves a streaming upload handler while honoring setup cancellation.
func StreamUploadHandlerContext(ctx context.Context, userID, driveID string) (func(context.Context, string, string, int64, io.Reader) error, error) {
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
	su, ok := d.(StreamUploader)
	if !ok {
		return nil, ErrNotImplemented
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return func(ctx context.Context, parentID, name string, size int64, reader io.Reader) error {
		opErr := su.UploadStream(ctx, c, parentID, name, size, reader)
		return withTokenPersist(opErr, c)
	}, nil
}
