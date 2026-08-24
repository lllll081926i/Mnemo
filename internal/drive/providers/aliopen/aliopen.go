// Package aliopen implements the Aliyun Drive Open API provider (AList-sourced).
package aliopen

import (
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const (
	apiHost        = "https://openapi.alipan.com"
	nativeAPIHost  = "https://api.aliyundrive.com"
	profileAPIHost = "https://api.alipan.com"
	oauthDefault   = "https://api.alistgo.com/alist/ali_open/token"
	RootID         = "aliopen_root"
	BackupRoot     = "backup_root"
	ResourceRoot   = "resource_root"

	defaultDownloadExpireSec = 14400
)

const (
	aliOpenMiB = int64(1024 * 1024)
	aliOpenGiB = int64(1024 * 1024 * 1024)

	// Profile is presentation metadata only. Keep an unavailable profile from
	// adding a request to every startup, while still allowing a later retry.
	aliOpenProfileSuccessInterval = 30 * 24 * time.Hour
	aliOpenProfileRetryInterval   = 24 * time.Hour
	aliOpenListPageLimit          = 100
)

const providerID = model.ProviderAliopen

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search":              true,
			"createShare":         true,
			"manageCreatedShares": true,
			"cancelCreatedShares": true,
			"shareExpiration":     true,
			"sharePassword":       true,
			"combinedShare":       true,
			"shareHistory":        true,
			"importShare":         true,
			"copy":                true,
			"recycleBin":          true,
			"permanentDelete":     true,
			"trashView":           false,
			"trashRestore":        false,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"sha1"}, []string{"sha1"}).SetConflictPolicies("refuse", "rename", "skip", "overwrite")
		}),
		Auth: authRefreshToken,
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "refresh_token", Type: "text", Label: "Refresh Token", Required: true, Placeholder: "粘贴阿里云盘 Open refresh_token"},
			{Key: "client_id", Type: "text", Label: "Client ID（可选）", Required: false},
			{Key: "client_secret", Type: "password", Label: "Client Secret（可选）", Required: false},
		}},
		Factory: func() drive.Driver { return &Driver{} },
	})
}
