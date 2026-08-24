// Package pan123 implements the 123 云盘 provider (AList-sourced web API,
// ported from the legacy Electron pan123 client).
package pan123

import (
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const (
	// RootID is the canonical root sentinel (mirrors legacy PAN123_ROOT).
	RootID = "pan123_root"

	apiMain         = "https://yun.123pan.com/b/api"
	apiSignIn       = "https://login.123pan.com/api/user/sign_in"
	apiUserInfo     = apiMain + "/user/info"
	apiFileList     = apiMain + "/file/list/new"
	apiDownloadInfo = apiMain + "/file/download_info"
	apiUploadReq    = apiMain + "/file/upload_request"
	apiUploadDone   = apiMain + "/file/upload_complete"
	apiUploadDoneV2 = apiMain + "/file/upload_complete/v2"
	// 注意：官方端点拼写即 s3_repare_upload_parts_batch（历史沿用）。
	apiS3Prepare  = apiMain + "/file/s3_repare_upload_parts_batch"
	apiS3Auth     = apiMain + "/file/s3_upload_object/auth"
	apiMove       = apiMain + "/file/mod_pid"
	apiRename     = apiMain + "/file/rename"
	apiTrash      = apiMain + "/file/trash"
	apiDelete     = apiMain + "/file/delete"
	apiShare      = apiMain + "/file/share_create" // 实际为 /share/create
	apiShareURL   = "https://www.123pan.com/a/api/share/create"
	apiShareGet   = apiMain + "/share/get"
	apiFileAsync  = apiMain + "/file/async"
	apiFileDetail = apiMain + "/file/info"

	// ua mirrors the legacy pan123 client user agent.
	ua       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
	referer  = "https://yun.123pan.com/"
	platform = "web"
	appVer   = "3"

	maxFilePool   = 8000
	listPageLimit = "50"
)

const providerID = model.ProviderPan123

// The current /a/api/share/create route requires an explicit expiration. The
// official web client represents a permanent link with this far-future value.
const pan123PermanentShareExpiration = "2099-12-12T08:00:00+08:00"

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search":          true,
			"createShare":     true,
			"shareExpiration": true,
			"sharePassword":   true,
			"combinedShare":   true,
			"shareHistory":    true,
			"importShare":     true,
			"trashView":       true,
			"trashRestore":    true,
			"recycleBin":      true,
			"copy":            false,
			"permanentDelete": true,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"md5"}, []string{"md5"})
		}),
		Auth: authLogin,
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "username", Type: "text", Label: "手机号/邮箱", Required: true, Hint: "123 云盘账号"},
			{Key: "password", Type: "password", Label: "密码", Required: true},
		}},
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// ---- driver ----

// Driver implements drive.Driver for 123 云盘.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return RootID }
