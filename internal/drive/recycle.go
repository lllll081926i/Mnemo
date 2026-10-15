package drive

import "context"

type TrashSnapshotItem struct {
	ID        string `json:"id"`
	IDKind    string `json:"idKind"`
	SiteID    string `json:"siteId"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Location  string `json:"location"`
	DeletedAt string `json:"deletedAt"`
}

type TrashSnapshot struct {
	Source    string              `json:"source"`
	Scope     string              `json:"scope"`
	Complete  bool                `json:"complete"`
	UpdatedAt int64               `json:"updatedAt"`
	Items     []TrashSnapshotItem `json:"items"`
}

type TrashWebsiteProvider interface {
	TrashWebsite(context.Context, Context) (string, error)
}
type EnterpriseTrashPreviewer interface {
	EnterpriseTrashPreview(context.Context, Context) (*TrashSnapshot, error)
}

func TrashWebsite(ctx context.Context, userID, driveID string) (website string, err error) {
	driver, account, err := driverAndCtx(userID, driveID)
	if err != nil {
		return "", err
	}
	defer func() { err = withTokenPersist(err, account) }()
	if provider, ok := driver.(TrashWebsiteProvider); ok {
		return provider.TrashWebsite(ctx, account)
	}
	return driver.Capabilities().TrashWebURL, nil
}

func PreviewEnterpriseTrash(ctx context.Context, userID, driveID string) (snapshot *TrashSnapshot, err error) {
	driver, account, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, account) }()
	if previewer, ok := driver.(EnterpriseTrashPreviewer); ok {
		return previewer.EnterpriseTrashPreview(ctx, account)
	}
	return nil, NotSupported("企业回收站只读试验")
}
