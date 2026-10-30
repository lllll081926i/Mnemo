package thunder

import (
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const RootID = "thunder_root"

type Driver struct{ drive.BaseDriver }

var _ drive.Driver = (*Driver)(nil)
var _ drive.OfflineDriver = (*Driver)(nil)

func init() {
	drive.Register(drive.Registration{
		ID: model.ProviderThunder, Meta: drive.GetMeta(model.ProviderThunder),
		Caps: drive.NewCapabilities(model.ProviderThunder, map[string]bool{
			"upload": true, "createTextFile": false, "offlineDownload": true,
			"cloudVideoPreview": true, "recycleBin": true,
			"trashView": true, "trashRestore": true, "permanentDelete": true,
			"createShare": true, "shareExpiration": true, "combinedShare": true,
			"shareHistory": true, "manageCreatedShares": true, "cancelCreatedShares": true,
		}, func(caps *drive.Capabilities) {
			caps.SetUploadMode(drive.UploadModeQueue)
			caps.UploadConflictPolicies = []string{"rename", "skip", "refuse"}
			caps.SetShareExpirationOptions(0, 1, 2, 3, 4, 5, 6, 7)
		}),
		Login: drive.LoginConfig{Notice: "手机号、密码或短信验证码发送至迅雷认证服务；人机验证使用迅雷官方页面。", Fields: []drive.LoginField{
			{Key: "login_mode", Default: "password", Purpose: "mode", Type: "select", Label: "登录方式", Required: true, Options: []drive.LoginOption{{Value: "password", Label: "密码登录"}, {Value: "sms", Label: "短信验证码登录"}}},
			{Key: "username", InputMode: "numeric", Purpose: "account", Type: "text", Label: "手机号", Required: true, Placeholder: "请输入迅雷账号绑定的手机号"},
			{Key: "password", RequiredModes: []string{"password"}, Modes: []string{"password"}, Purpose: "password", Type: "password", Label: "密码", Required: false, Placeholder: "请输入迅雷账号密码", Hint: "密码仅用于本次登录，不保存在本机；需要安全验证时会打开官方验证页面"},
			{Key: "sms_code", RequiredModes: []string{"sms"}, Modes: []string{"sms"}, InputMode: "numeric", Purpose: "sms", Type: "text", Label: "短信验证码", Required: false, Placeholder: "请输入 6 位短信验证码"},
		}},
		Auth: authLogin, Factory: func() drive.Driver { return &Driver{} },
	})
}

func (*Driver) ID() string                       { return model.ProviderThunder }
func (*Driver) Meta() drive.Meta                 { return drive.GetMeta(model.ProviderThunder) }
func (*Driver) Capabilities() drive.Capabilities { return drive.Require(model.ProviderThunder).Caps }
func (*Driver) RootID() string                   { return RootID }
