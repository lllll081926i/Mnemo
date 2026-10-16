package drive_test

import (
	"testing"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers"
	"mnemo-go/internal/model"
)

func TestRegisteredLoginAndCapabilityContracts(testContext *testing.T) {
	for _, registration := range drive.All() {
		testContext.Run(registration.ID, func(testContext *testing.T) {
			for _, field := range registration.Login.Fields {
				if field.Type != "select" && field.Purpose == "" {
					testContext.Errorf("登录字段缺少用途声明：%s", field.Key)
				}
				if field.Hidden && field.Required {
					testContext.Errorf("隐藏字段不能作为必填输入：%s", field.Key)
				}
			}
			account := &model.Account{Token: &model.TokenInfo{TokenFrom: registration.ID}}
			snapshot := drive.CapabilitiesForAccount(account)
			if registration.Caps.Upload && snapshot.States["upload"].State != "conditional" {
				testContext.Fatal("上传不能在未验证权限时标为无条件可用")
			}
			account.AuthState = "reauth_required"
			if drive.CapabilitiesForAccount(account).States["authentication"].State != "unsupported" {
				testContext.Fatal("账号失效状态未覆盖")
			}
		})
	}
}

func TestAccountCapabilityRestrictionsDoNotDisableSupportedOperations(testContext *testing.T) {
	for _, provider := range []string{"s3", "webdav", "pan139", "pan189", "lanzou", "guangya", "thunder", "pikpak"} {
		snapshot := drive.CapabilitiesForAccount(&model.Account{Token: &model.TokenInfo{TokenFrom: provider}})
		if len(snapshot.Overrides) != 0 || len(snapshot.States) == 0 {
			testContext.Fatalf("%s 不应将未验证权限直接变成关闭功能", provider)
		}
	}
}
