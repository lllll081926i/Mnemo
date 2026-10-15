package onedrive

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const testRecycleSiteID = "tenant.sharepoint.com,11111111-1111-1111-1111-111111111111,22222222-2222-2222-2222-222222222222"

func recycleMetadata(request *http.Request) *http.Response {
	switch request.URL.Path {
	case "/v1.0/me/drive":
		return onedriveResponse(request, 200, `{"id":"drive","driveType":"business","webUrl":"https://tenant.sharepoint.com/sites/team/Documents"}`)
	case "/v1.0/me/drive/root":
		return onedriveResponse(request, 200, `{"parentReference":{"driveId":"drive"},"sharepointIds":{"siteId":"11111111-1111-1111-1111-111111111111","webId":"22222222-2222-2222-2222-222222222222"}}`)
	default:
		return onedriveResponse(request, 200, `{"id":"`+testRecycleSiteID+`","webUrl":"https://tenant.sharepoint.com/sites/team"}`)
	}
}

func TestEnterpriseTrashSnapshotIsAtomicAndReadOnly(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { netx.TestTransportHook = previous })
	for _, scenario := range []string{"complete", "forbidden", "throttled", "foreign", "duplicate", "missingValue"} {
		testContext.Run(scenario, func(testContext *testing.T) {
			pages := 0
			netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Host != "graph.microsoft.com" {
					testContext.Fatalf("越界或写入请求: %s %s", request.Method, request.URL)
				}
				if !strings.Contains(request.URL.Path, "/recycleBin/") {
					return recycleMetadata(request), nil
				}
				pages++
				if pages == 1 {
					next := "https://graph.microsoft.com/beta/next"
					if scenario == "foreign" {
						next = "https://evil.test/next"
					}
					return onedriveResponse(request, 200, `{"value":[{"id":"item","name":"原文件"}],"@odata.nextLink":"`+next+`"}`), nil
				}
				return nil, errors.New("非预期路径")
			})
			baseTransport := netx.TestTransportHook
			netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != "/beta/next" {
					return baseTransport.RoundTrip(request)
				}
				switch scenario {
				case "forbidden":
					return onedriveResponse(request, 403, `{"error":{"code":"accessDenied"}}`), nil
				case "throttled":
					response := onedriveResponse(request, 429, `{}`)
					response.Header.Set("Retry-After", "40")
					return response, nil
				case "duplicate":
					return onedriveResponse(request, 200, `{"value":[{"id":"item"}]}`), nil
				case "missingValue":
					return onedriveResponse(request, 200, `{}`), nil
				default:
					return onedriveResponse(request, 200, `{"value":[]}`), nil
				}
			})
			snapshot, err := (&Driver{}).EnterpriseTrashPreview(testContext.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "secret", ProviderDriveType: "business"}})
			if scenario == "complete" {
				if err != nil || snapshot == nil || !snapshot.Complete || len(snapshot.Items) != 1 || snapshot.UpdatedAt == 0 {
					testContext.Fatalf("快照无效: %+v %v", snapshot, err)
				}
			} else if err == nil || snapshot != nil {
				testContext.Fatalf("失败返回不完整快照: %+v %v", snapshot, err)
			}
			if scenario == "throttled" && !strings.Contains(err.Error(), "40") {
				testContext.Fatal("缺少冷却提示")
			}
		})
	}
}

func TestRecycleCapabilityAndURLIsolation(testContext *testing.T) {
	for _, providerType := range []string{"personal", "business", "documentLibrary", ""} {
		capabilities := (&Driver{}).AccountCapabilities(drive.Context{Token: &model.TokenInfo{ProviderDriveType: providerType}})
		if capabilities.Overrides["trashView"] || capabilities.Overrides["trashPurge"] || capabilities.Overrides["trashRestore"] != (providerType == "personal") {
			testContext.Fatalf("错误开启能力: %+v", capabilities)
		}
	}
	for _, target := range []string{"https://tenant.sharepoint.com.evil.test/sites/a", "https://user@tenant.sharepoint.com/sites/a", "http://tenant.sharepoint.com/", "https://tenant.sharepoint.com:8443/", "https://tenant.sharepoint.com/#secret"} {
		if _, err := trustedSharepointURL(target); err == nil {
			testContext.Errorf("接受不可信站点: %s", target)
		}
	}
}

func TestPersonalRestoreRejectsMismatchedReceipt(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = onedriveRoundTripper(func(request *http.Request) (*http.Response, error) {
		return onedriveResponse(request, 200, `{"id":"other"}`), nil
	})
	completed, err := (&Driver{}).Restore(testContext.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "secret", ProviderDriveType: "personal"}}, []string{"file"})
	if err == nil || len(completed) != 0 {
		testContext.Fatalf("错误回执被视为成功: %v %v", completed, err)
	}
}
