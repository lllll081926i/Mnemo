package drive

import (
	"context"
	"mnemo-go/internal/model"
)

type ShareReceiptDriver interface {
	SaveShareReceipt(context.Context, Context, *ShareImportSession, []string, string) ([]model.ShareImportItem, error)
}

type ShareSessionValidator interface {
	ValidateShareSession(*ShareImportSession) error
}

type ShareReceiptVerifier interface {
	VerifyShareReceipt(context.Context, Context, []model.ShareImportItem) ([]model.ShareImportItem, error)
}

func RefreshImportedShareReceipt(ctx context.Context, userID, driveID, provider string, items []model.ShareImportItem) (result []model.ShareImportItem, err error) {
	driver, operation, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, operation) }()
	if operation.TokenFrom != provider {
		return nil, ErrNotImplemented
	}
	verifier, ok := driver.(ShareReceiptVerifier)
	if !ok {
		return nil, ErrNotImplemented
	}
	return verifier.VerifyShareReceipt(ctx, operation, items)
}

func AcceptedShareIDs(items []model.ShareImportItem) []string {
	var ids []string
	for _, item := range items {
		if item.Status == "submitted" || item.Status == "completed" {
			ids = append(ids, item.SourceID)
		}
	}
	return ids
}
