package onedrive

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (*Driver) AccountCapabilities(account drive.Context) drive.AccountCapabilitySnapshot {
	personal := account.Token != nil && account.Token.ProviderDriveType == "personal"
	return drive.AccountCapabilitySnapshot{
		Overrides: map[string]bool{"trashView": false, "trashRestore": personal, "trashPurge": false, "trashClear": false, "permanentDelete": false, "localDeletionHistory": true},
		States: map[string]model.CapabilityState{
			"trashView":              {State: "unsupported", Reason: "不提供默认完整云端回收站列表；本机删除记录仅覆盖 Mnemo 操作"},
			"trashRestore":           {State: map[bool]string{true: "conditional", false: "unsupported"}[personal], Reason: "仅个人版持有 driveItem ID 的恢复；企业版请使用官网", Requires: []string{"known_drive_item_id", "personal_account"}},
			"trashPurge":             {State: "unsupported", Reason: "未开放永久删除或清空回收站"},
			"enterpriseTrashPreview": {State: "conditional", Reason: "需用户主动读取 Graph beta 一级回收站；不包含二级，不开放恢复写入", Requires: []string{"business_account", "trusted_site_metadata", "tenant_permission"}},
		},
	}
}

type recycleSite struct {
	ID      string
	DriveID string
	WebURL  *url.URL
}

var sharepointGUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func trustedSharepointURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Port() != "" && parsed.Port() != "443" || !strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".sharepoint.com") {
		return nil, errors.New("OneDrive 未返回可信的 SharePoint 站点地址")
	}
	return parsed, nil
}

func (client *client) recycleSite(ctx context.Context, expectedDrive string) (*recycleSite, error) {
	var current struct {
		ID        string `json:"id"`
		DriveType string `json:"driveType"`
		WebURL    string `json:"webUrl"`
	}
	if err := client.getJSON(ctx, "/me/drive?$select=id,driveType,webUrl", &current); err != nil {
		return nil, err
	}
	if current.ID == "" || current.DriveType != "business" && current.DriveType != "documentLibrary" {
		return nil, errors.New("当前账号没有可确认的企业文档库")
	}
	if expectedDrive != "" && current.ID != model.StripDriveID(providerID, expectedDrive) {
		return nil, errors.New("Graph 返回的文档库不属于当前挂载空间")
	}
	driveURL, err := trustedSharepointURL(current.WebURL)
	if err != nil {
		return nil, err
	}
	var root struct {
		ParentReference struct {
			DriveID string `json:"driveId"`
		} `json:"parentReference"`
		SharepointIDs struct {
			SiteID  string `json:"siteId"`
			WebID   string `json:"webId"`
			SiteURL string `json:"siteUrl"`
		} `json:"sharepointIds"`
	}
	if err := client.getJSON(ctx, "/me/drive/root?$select=parentReference,sharepointIds", &root); err != nil {
		return nil, err
	}
	if root.ParentReference.DriveID != "" && root.ParentReference.DriveID != current.ID || !sharepointGUID.MatchString(root.SharepointIDs.SiteID) || !sharepointGUID.MatchString(root.SharepointIDs.WebID) {
		return nil, errors.New("SharePoint 站点与当前文档库身份无法确认")
	}
	siteID := driveURL.Hostname() + "," + root.SharepointIDs.SiteID + "," + root.SharepointIDs.WebID
	var site struct {
		ID     string `json:"id"`
		WebURL string `json:"webUrl"`
	}
	if err := client.getJSON(ctx, "/sites/"+url.PathEscape(siteID)+"?$select=id,webUrl", &site); err != nil {
		return nil, err
	}
	parsed, err := trustedSharepointURL(site.WebURL)
	if err != nil || !strings.EqualFold(site.ID, siteID) || !strings.EqualFold(parsed.Hostname(), driveURL.Hostname()) {
		return nil, errors.New("SharePoint 站点元数据不属于当前账号文档库")
	}
	return &recycleSite{ID: site.ID, DriveID: current.ID, WebURL: parsed}, nil
}

func (*Driver) TrashWebsite(ctx context.Context, account drive.Context) (string, error) {
	client, err := clientOf(account)
	if err != nil {
		return "", err
	}
	if account.Token.ProviderDriveType == "personal" {
		return "https://onedrive.live.com/?qt=recyclebin", nil
	}
	site, err := client.recycleSite(ctx, account.DriveID)
	if isGraphAuthenticationFailure(err) {
		client, err = refreshedClientAfterGraphAuthFailure(ctx, account, err)
		if err == nil {
			site, err = client.recycleSite(ctx, account.DriveID)
		}
	}
	if err != nil {
		return "", fmt.Errorf("无法确认当前站点回收站，请在微软官网选择账号: %w", err)
	}
	website := *site.WebURL
	website.Path = strings.TrimRight(website.Path, "/") + "/_layouts/15/RecycleBin.aspx"
	website.RawPath, website.RawQuery, website.Fragment = "", "", ""
	return website.String(), nil
}

func (*Driver) EnterpriseTrashPreview(ctx context.Context, account drive.Context) (*drive.TrashSnapshot, error) {
	if account.Token == nil || account.Token.ProviderDriveType != "business" && account.Token.ProviderDriveType != "documentLibrary" {
		return nil, drive.NotSupported("仅企业文档库可试验读取一级回收站")
	}
	client, err := clientOf(account)
	if err != nil {
		return nil, err
	}
	site, err := client.recycleSite(ctx, account.DriveID)
	refreshed := false
	if isGraphAuthenticationFailure(err) {
		refreshed = true
		client, err = refreshedClientAfterGraphAuthFailure(ctx, account, err)
		if err == nil {
			site, err = client.recycleSite(ctx, account.DriveID)
		}
	}
	if err != nil {
		return nil, err
	}
	result := &drive.TrashSnapshot{Source: "graph_beta_level1", Scope: "仅当前站点一级回收站；不包含二级；实验性只读", Items: []drive.TrashSnapshotItem{}}
	next := "https://graph.microsoft.com/beta/sites/" + url.PathEscape(site.ID) + "/recycleBin/items?$top=200"
	seen := map[string]bool{}
	itemIDs := map[string]bool{}
	for page := 0; next != "" && page < 200; page++ {
		if seen[next] || !trustedGraphURL(next) {
			return nil, errors.New("企业回收站分页重复或来源不可信")
		}
		seen[next] = true
		var response struct {
			Value []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Size      int64  `json:"size"`
				DeletedAt string `json:"deletedDateTime"`
				Location  string `json:"deletedFromLocation"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		requestErr := client.getJSON(ctx, next, &response)
		if isGraphAuthenticationFailure(requestErr) && !refreshed {
			refreshed = true
			client, requestErr = refreshedClientAfterGraphAuthFailure(ctx, account, requestErr)
			if requestErr == nil {
				requestErr = client.getJSON(ctx, next, &response)
			}
		}
		if requestErr != nil {
			return nil, requestErr
		}
		if response.Value == nil {
			return nil, errors.New("企业回收站列表缺少 value，未视为空列表")
		}
		for _, item := range response.Value {
			if item.ID == "" || itemIDs[item.ID] {
				return nil, errors.New("企业回收站条目 ID 缺失或重复，快照未更新")
			}
			itemIDs[item.ID] = true
			result.Items = append(result.Items, drive.TrashSnapshotItem{ID: item.ID, IDKind: "recycleBinItem", SiteID: site.ID, Name: item.Name, Size: item.Size, Location: item.Location, DeletedAt: item.DeletedAt})
		}
		next = response.NextLink
	}
	if next != "" {
		return nil, errors.New("企业回收站分页达到上限，未返回不完整快照")
	}
	result.Complete = true
	result.UpdatedAt = time.Now().Unix()
	return result, nil
}
