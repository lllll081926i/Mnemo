package aliopen

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func aliOpenNotFound(err error) bool {
	var requestErr *aliOpenRequestError
	return errors.As(err, &requestErr) && requestErr.StatusCode == http.StatusNotFound
}

// CreateShare creates a share link.
func (c *client) CreateShare(ctx context.Context, scope Scope, fileIDs []string, shareName, expiration, password string) (*model.ShareItem, error) {
	body := map[string]any{
		"drive_id":     c.scopedDriveID(scope),
		"file_id_list": fileIDs,
		"share_name":   shareName,
		"share_pwd":    password,
		"expiration":   expiration,
		"description":  shareName,
	}
	var share struct {
		ShareID    string `json:"share_id"`
		ShareURL   string `json:"share_url"`
		ShareMsg   string `json:"share_msg"`
		Expiration string `json:"expiration"`
		Status     string `json:"status"`
		DriveID    string `json:"drive_id"`
	}
	err := c.apiPost(ctx, "/adrive/v1.0/openFile/createShareLink", body, &share)
	if err != nil && aliOpenNotFound(err) {
		// Some personal-drive regions no longer expose the legacy Open endpoint
		// but retain the compatible consumer route. Only fall back on a definite
		// not-found result: retrying another endpoint after any other failure
		// could create duplicate links.
		err = c.apiPostAt(ctx, nativeAPIHost, "/adrive/v2/share_link/create", body, &share)
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(share.ShareURL) == "" && strings.TrimSpace(share.ShareID) != "" {
		share.ShareURL = "https://www.alipan.com/s/" + share.ShareID
	}
	return &model.ShareItem{
		ShareID: share.ShareID, ShareURL: share.ShareURL, ShareMsg: share.ShareMsg,
		Expiration: share.Expiration, Status: share.Status, DriveID: share.DriveID,
		ShareName: shareName, SharePwd: password,
	}, nil
}

// CancelShare revokes a created share remotely. The legacy Open API is kept
// first for accounts where creation still uses it; regions that retired that
// route use the compatible consumer API instead.
func (c *client) CancelShare(ctx context.Context, shareID string) error {
	shareID = strings.TrimSpace(shareID)
	if shareID == "" {
		return errors.New("aliopen: 取消分享缺少分享标识")
	}
	body := map[string]any{"share_id": shareID}
	err := c.apiPost(ctx, "/adrive/v1.0/openFile/cancelShareLink", body, nil)
	if err != nil && aliOpenNotFound(err) {
		err = c.apiPostAt(ctx, nativeAPIHost, "/adrive/v2/share_link/cancel", body, nil)
	}
	return err
}

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if len(params.FileIDs) == 0 {
		return nil, errors.New("aliopen: 创建分享至少选择一个文件")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	scope, err := sameAliOpenScopeIDs(params.FileIDs...)
	if err != nil {
		return nil, err
	}
	fids := make([]string, 0, len(params.FileIDs))
	for _, id := range params.FileIDs {
		ref := parseRef(id)
		fids = append(fids, ref.FID)
	}
	item, err := cl.CreateShare(ctx, scope, fids, params.ShareName, params.Expiration, params.Password)
	if err != nil {
		return nil, err
	}
	if item == nil || (item.ShareID == "" && item.ShareURL == "" && item.ShareMsg == "" && item.FullShareMsg == "") {
		return nil, errors.New("aliopen: 创建分享未返回链接")
	}
	item.AccountID = c.UserID
	item.DriveID = c.DriveID
	item.FileID = params.FileIDs[0]
	item.FileIDList = append([]string(nil), params.FileIDs...)
	return item, nil
}

func (d *Driver) CancelShare(ctx context.Context, c drive.Context, share model.ShareHistoryEntry) error {
	shareID := strings.TrimSpace(share.ShareID)
	if shareID == "" {
		shareID, _ = parseAliShareURL(share.ShareURL)
	}
	if shareID == "" {
		return errors.New("aliopen: 取消分享缺少分享标识")
	}
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	return cl.CancelShare(ctx, shareID)
}

// ---- Share Import (importShare capability) ----

// shareListResp is the aliopen listByShare response.
type shareListResp struct {
	Items      []aliFile `json:"items"`
	NextMarker string    `json:"next_marker"`
}

// ImportShareSession implements drive.ShareImportDriver.
func (d *Driver) ImportShareSession(ctx context.Context, c drive.Context, shareURL, password string) (*drive.ShareImportSession, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	shareID, _ := parseAliShareURL(shareURL)
	if shareID == "" {
		return nil, errors.New("aliopen: 无效的分享链接")
	}
	// Step 1: getShareToken
	var tokenResp struct {
		ShareToken string `json:"share_token"`
	}
	body := map[string]any{"share_id": shareID}
	if password != "" {
		body["share_pwd"] = password
	}
	if err := cl.apiPost(ctx, "/adrive/v1.0/openFile/getShareToken", body, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.ShareToken == "" {
		return nil, errors.New("获取分享凭证失败（链接无效、已取消或提取码错误）")
	}
	// Step 2: listByShare
	hdrs := map[string]string{"x-share-token": tokenResp.ShareToken}
	files, err := aliShareListAll(ctx, cl, shareID, tokenResp.ShareToken, "root", hdrs)
	if err != nil {
		return nil, err
	}
	return &drive.ShareImportSession{
		Provider:   providerID,
		ShareURL:   shareURL,
		ShareID:    shareID,
		Password:   password,
		ShareToken: tokenResp.ShareToken,
		RootFileID: "root",
		Files:      files,
	}, nil
}

func aliShareListAll(ctx context.Context, cl *client, shareID, shareToken, parentFileID string, hdrs map[string]string) ([]drive.ShareImportFile, error) {
	var out []drive.ShareImportFile
	marker := ""
	seenMarkers := map[string]bool{}
	for {
		body := map[string]any{
			"share_id":        shareID,
			"parent_file_id":  parentFileID,
			"limit":           100,
			"order_by":        "name",
			"order_direction": "ASC",
		}
		if marker != "" {
			body["marker"] = marker
		}
		var resp shareListResp
		if err := cl.apiPostWith(ctx, "/adrive/v1.0/openFile/listByShare", body, &resp, hdrs); err != nil {
			return nil, err
		}
		for i := range resp.Items {
			item := resp.Items[i]
			out = append(out, drive.ShareImportFile{
				FileID: item.FileID,
				Name:   item.Name,
				Size:   item.Size,
				IsDir:  item.Type == "folder",
			})
		}
		if resp.NextMarker == "" {
			break
		}
		if seenMarkers[resp.NextMarker] {
			return nil, errors.New("aliopen: duplicate share list cursor")
		}
		seenMarkers[resp.NextMarker] = true
		marker = resp.NextMarker
	}
	return out, nil
}

// SaveShare implements drive.ShareImportDriver.
func (d *Driver) SaveShare(ctx context.Context, c drive.Context, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	if session == nil || session.Provider != providerID || strings.TrimSpace(session.ShareID) == "" || strings.TrimSpace(session.ShareToken) == "" {
		return nil, errors.New("aliopen: 分享会话无效或已过期")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	// Determine target drive + parent
	targetDrive := cl.session.ResourceDriveID
	if targetDrive == "" {
		targetDrive = cl.session.DriveID
	}
	if targetDrive == "" {
		return nil, errors.New("aliopen: 缺少目标 drive_id")
	}
	parent := toParentID
	if parent == "" || parent == "root" || parent == "/" || parent == RootID {
		parent = "root"
	}
	ref := parseRef(parent)
	parent = ref.FID
	if parent == "" {
		parent = "root"
	}
	// If the target scope's drive_id differs from targetDrive, use the target drive
	if ref.Scope == ScopeResource && cl.session.ResourceDriveID != "" {
		targetDrive = cl.session.ResourceDriveID
	}
	hdrs := map[string]string{"x-share-token": session.ShareToken}
	var saved []string
	var failed []error
	for _, fileID := range fileIDs {
		body := map[string]any{
			"share_id":          session.ShareID,
			"file_id":           fileID,
			"to_drive_id":       targetDrive,
			"to_parent_file_id": parent,
			"auto_rename":       true,
		}
		if err := cl.apiPostWith(ctx, "/adrive/v1.0/openFile/copy", body, nil, hdrs); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", fileID, err))
			continue
		}
		saved = append(saved, fileID)
	}
	return saved, errors.Join(failed...)
}

// parseAliShareURL extracts share_id from an Aliyun share URL.
func parseAliShareURL(raw string) (shareID string, pwd string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "//") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	// https://www.alipan.com/s/{shareID}
	// https://www.aliyundrive.com/s/{shareID}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "s" {
		shareID = parts[1]
	}
	if shareID == "" && len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 8 {
			shareID = last
		}
	}
	pwd = u.Query().Get("p")
	if pwd == "" {
		pwd = u.Query().Get("pwd")
	}
	return shareID, pwd
}
