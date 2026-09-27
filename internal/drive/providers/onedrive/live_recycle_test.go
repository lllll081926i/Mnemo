package onedrive

import (
	"errors"
	"net/url"
	"os"
	"testing"

	"mnemo-go/internal/config"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestLiveBusinessRecycleListReadOnly(t *testing.T) {
	if os.Getenv("MNEMO_LIVE_ONEDRIVE_READONLY") != "1" {
		t.Skip("仅在明确启用 OneDrive 只读诊断时运行")
	}
	configDir, err := config.UserConfigDir("Mnemo")
	if err != nil {
		t.Fatal(err)
	}
	accountStore, err := store.Open(configDir)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := accountStore.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.Provider() != model.ProviderOnedrive || account.Token == nil || account.Disabled {
			continue
		}
		token := *account.Token
		if token.ProviderDriveType != "business" {
			t.Logf("跳过账号类型=%q", token.ProviderDriveType)
			continue
		}
		client := newClient(token.AccessToken)
		var driveInfo struct {
			WebURL string `json:"webUrl"`
		}
		err := client.getJSON(t.Context(), "/me/drive?$select=webUrl", &driveInfo)
		if isGraphAuthenticationFailure(err) {
			if refreshErr := refreshOneDriveAccessToken(t.Context(), &token); refreshErr != nil {
				t.Logf("令牌刷新失败类型=%T", refreshErr)
				continue
			}
			client = newClient(token.AccessToken)
			err = client.getJSON(t.Context(), "/me/drive?$select=webUrl", &driveInfo)
		}
		if err != nil {
			t.Logf("Drive 信息读取失败类型=%T", err)
			continue
		}
		var root struct {
			SharePointIDs struct {
				SiteID string `json:"siteId"`
				WebID  string `json:"webId"`
			} `json:"sharepointIds"`
		}
		if err := client.getJSON(t.Context(), "/me/drive/root?$select=sharepointIds", &root); err != nil {
			t.Logf("SharePoint ID 读取失败类型=%T", err)
			continue
		}
		parsed, err := url.Parse(driveInfo.WebURL)
		if err != nil || parsed.Hostname() == "" || root.SharePointIDs.SiteID == "" || root.SharePointIDs.WebID == "" {
			t.Logf("SharePoint 站点元数据不完整：host=%t,site=%t,web=%t", parsed != nil && parsed.Hostname() != "", root.SharePointIDs.SiteID != "", root.SharePointIDs.WebID != "")
			continue
		}
		siteID := parsed.Hostname() + "," + root.SharePointIDs.SiteID + "," + root.SharePointIDs.WebID
		var response struct {
			Value []struct {
				ID string `json:"id"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		err = client.getJSON(t.Context(), "https://graph.microsoft.com/beta/sites/"+url.PathEscape(siteID)+"/recycleBin/items?$top=20", &response)
		if err != nil {
			var graphErr *graphAPIError
			if errors.As(err, &graphErr) {
				t.Logf("Graph beta 回收站列表 HTTP=%d，代码=%q", graphErr.StatusCode, graphErr.Code)
			} else {
				t.Logf("Graph beta 回收站列表失败类型=%T", err)
			}
			continue
		}
		t.Logf("Graph beta 回收站列表成功：条目数=%d，后续分页=%t", len(response.Value), response.NextLink != "")
		return
	}
	t.Skip("未找到可读的 OneDrive 工作或学校账号")
}

func TestLivePersonalRecycleChildrenProbeReadOnly(t *testing.T) {
	if os.Getenv("MNEMO_LIVE_ONEDRIVE_READONLY") != "1" {
		t.Skip("仅在明确启用 OneDrive 只读诊断时运行")
	}
	configDir, err := config.UserConfigDir("Mnemo")
	if err != nil {
		t.Fatal(err)
	}
	accountStore, err := store.Open(configDir)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := accountStore.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	onedriveAccounts := 0
	for _, account := range accounts {
		if account.Provider() != model.ProviderOnedrive {
			continue
		}
		onedriveAccounts++
		accountType := ""
		if account.Token != nil {
			accountType = account.Token.ProviderDriveType
		}
		t.Logf("OneDrive 账号 #%d：类型=%q，禁用=%t，标识末尾=%s", onedriveAccounts, accountType, account.Disabled, maskedAccountSuffix(account.UserID))
		if account.Token == nil || account.Disabled || account.Token.ProviderDriveType != "personal" {
			continue
		}
		token := *account.Token
		client := newClient(token.AccessToken)
		var driveInfo struct {
			WebURL string `json:"webUrl"`
			Quota  struct {
				Deleted int64 `json:"deleted"`
			} `json:"quota"`
		}
		err := client.getJSON(t.Context(), "/me/drive?$select=quota,webUrl", &driveInfo)
		if isGraphAuthenticationFailure(err) {
			if refreshErr := refreshOneDriveAccessToken(t.Context(), &token); refreshErr != nil {
				t.Logf("令牌刷新失败类型=%T", refreshErr)
				continue
			}
			client = newClient(token.AccessToken)
			err = client.getJSON(t.Context(), "/me/drive?$select=quota,webUrl", &driveInfo)
		}
		if err != nil {
			t.Logf("个人版容量读取失败类型=%T", err)
			continue
		}
		path := "/me/drive/root/children?includeDeletedItems=true&$select=id,deleted&$top=200"
		ordinary, deleted := 0, 0
		for page := 0; page < 50 && path != ""; page++ {
			var response struct {
				Value []struct {
					Deleted *struct{} `json:"deleted"`
				} `json:"value"`
				NextLink string `json:"@odata.nextLink"`
			}
			if err := client.getJSON(t.Context(), path, &response); err != nil {
				t.Logf("SimpleList 列表请求失败类型=%T", err)
				break
			}
			for _, item := range response.Value {
				if item.Deleted == nil {
					ordinary++
				} else {
					deleted++
				}
			}
			if response.NextLink != "" && !trustedGraphURL(response.NextLink) {
				t.Fatal("Graph 返回不可信的分页链接")
			}
			path = response.NextLink
		}
		t.Logf("个人版已删容量=%d，SimpleList 路径普通条目=%d，deleted 条目=%d", driveInfo.Quota.Deleted, ordinary, deleted)
		var root struct {
			SharePointIDs struct {
				SiteID string `json:"siteId"`
				WebID  string `json:"webId"`
			} `json:"sharepointIds"`
		}
		if err := client.getJSON(t.Context(), "/me/drive/root?$select=sharepointIds", &root); err != nil {
			t.Logf("个人站点 ID 读取失败类型=%T", err)
			continue
		}
		parsed, parseErr := url.Parse(driveInfo.WebURL)
		if parseErr != nil || parsed.Hostname() == "" || root.SharePointIDs.SiteID == "" || root.SharePointIDs.WebID == "" {
			t.Logf("个人站点元数据不完整：host=%t,site=%t,web=%t", parseErr == nil && parsed.Hostname() != "", root.SharePointIDs.SiteID != "", root.SharePointIDs.WebID != "")
			continue
		}
		siteID := parsed.Hostname() + "," + root.SharePointIDs.SiteID + "," + root.SharePointIDs.WebID
		var recycle struct {
			Value []struct {
				ID string `json:"id"`
			} `json:"value"`
		}
		err = client.getJSON(t.Context(), "https://graph.microsoft.com/beta/sites/"+url.PathEscape(siteID)+"/recycleBin/items?$top=20", &recycle)
		if err != nil {
			var graphErr *graphAPIError
			if errors.As(err, &graphErr) {
				t.Logf("个人站点 Graph beta 回收站 HTTP=%d，代码=%q", graphErr.StatusCode, graphErr.Code)
			} else {
				t.Logf("个人站点 Graph beta 回收站读取失败类型=%T", err)
			}
			continue
		}
		t.Logf("个人站点 Graph beta 回收站条目=%d", len(recycle.Value))
	}
	if onedriveAccounts == 0 {
		t.Skip("本机没有 OneDrive 账号")
	}
}

func maskedAccountSuffix(value string) string {
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}
