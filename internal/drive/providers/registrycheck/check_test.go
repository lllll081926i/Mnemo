package registrycheck

import (
	"testing"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers"
	"mnemo-go/internal/model"
)

func TestAllProvidersRegistered(t *testing.T) {
	want := []string{"aliopen", "dropbox", "guangya", "ilanzou", "lanzou", "onedrive", "pan123", "pan139", "pan189", "pikpak", "s3", "thunder", "webdav", "yike"}
	if drive.Count() != len(want) {
		t.Fatalf("registered %d providers, want %d", drive.Count(), len(want))
	}
	seen := map[string]bool{}
	for _, p := range drive.All() {
		seen[p.ID] = true
		if p.Caps.Provider != p.ID {
			t.Errorf("caps provider mismatch for %s", p.ID)
		}
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("provider %s not registered", w)
		}
	}
	// gofile/gdrive must NOT be present
	if seen["gofile"] || seen["gdrive"] {
		t.Error("gofile/gdrive must be removed")
	}
	_ = model.ProviderUnknown
}

func TestShareCapabilitiesMatchImplementedAdapters(t *testing.T) {
	creation := map[string]bool{"pikpak": true, "aliopen": true, "pan123": true, "pan139": true, "pan189": true, "guangya": true, "onedrive": true, "dropbox": true, "lanzou": true, "s3": true}
	for _, registration := range drive.All() {
		adapter := registration.Factory()
		_, imports := adapter.(drive.ShareImportDriver)
		_, cancels := adapter.(drive.ShareCancellationDriver)
		for _, caps := range []drive.Capabilities{registration.Caps, adapter.Capabilities(), drive.RegistryCaps(registration.ID)} {
			if caps.CreateShare != creation[registration.ID] || caps.ImportShare != imports || caps.CancelCreatedShares != cancels {
				t.Errorf("%s 分享能力与实现不符: %+v", registration.ID, caps)
			}
		}
		if _, streams := adapter.(drive.StreamUploader); streams {
			t.Errorf("%s 已新增流上传实现，应补充真实目标 ID 契约后重新审计迁移", registration.ID)
		}
	}
}

func TestColdFreshLookupsCannotReturnPlaceholderMetadata(testContext *testing.T) {
	for _, provider := range []string{"lanzou", "ilanzou", "pan189"} {
		testContext.Run(provider, func(testContext *testing.T) {
			fresh, ok := drive.New(provider).(drive.FreshFileDriver)
			if !ok {
				testContext.Fatal("缺少冷启动校验保护")
			}
			file, err := fresh.GetFileFresh(testContext.Context(), drive.Context{}, "uncached-file")
			if err == nil || file != nil {
				testContext.Fatalf("占位大小 0 不能作为真实文件证据: %+v, %v", file, err)
			}
		})
	}
}

func TestFavoriteCapabilitiesMatchCompleteNativeAdapters(t *testing.T) {
	native := map[string]bool{"pikpak": true, "onedrive": true, "pan123": true, "aliopen": true}
	for _, p := range drive.All() {
		_, implemented := p.Factory().(drive.RemoteFavorites)
		if implemented != native[p.ID] || p.Caps.Favorite != native[p.ID] {
			t.Errorf("%s favorite capability=%v adapter=%v", p.ID, p.Caps.Favorite, implemented)
		}
	}
}

func TestOfflineCapabilitiesMatchCompleteAdapters(testContext *testing.T) {
	for _, registration := range drive.All() {
		testContext.Run(registration.ID, func(testContext *testing.T) {
			adapter := registration.Factory()
			_, implemented := adapter.(drive.OfflineDriver)
			if registration.Caps.OfflineDownload != implemented {
				testContext.Errorf("云离线注册开关=%v，完整实现=%v", registration.Caps.OfflineDownload, implemented)
			}
			if adapter.Capabilities().OfflineDownload != implemented {
				testContext.Errorf("驱动的云离线开关与完整实现不一致")
			}
			if drive.RegistryCaps(registration.ID).OfflineDownload != implemented {
				testContext.Errorf("门面的云离线开关与完整实现不一致")
			}
		})
	}
}
