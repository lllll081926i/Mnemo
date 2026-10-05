package thunder

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

var _ drive.ShareCancellationDriver = (*Driver)(nil)

func shareExpirationDays(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" || value == "-1" {
		return -1, nil
	}
	days, err := strconv.Atoi(value)
	if err != nil {
		expires, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			expires, parseErr = time.Parse("2006-01-02", value)
		}
		if parseErr != nil || !expires.After(time.Now()) {
			return 0, errors.New("迅雷分享有效期格式无效或已过期")
		}
		days = int((time.Until(expires) + 24*time.Hour - 1) / (24 * time.Hour))
	}
	if days < 1 || days > 7 {
		return 0, errors.New("迅雷分享仅支持永久或 1–7 天有效期")
	}
	return days, nil
}

func (*Driver) CreateShare(ctx context.Context, account drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if err := validateIDs(params.FileIDs); err != nil {
		return nil, err
	}
	for _, id := range params.FileIDs {
		if id == RootID || id == "root" || id == "/" {
			return nil, errors.New("迅雷不支持分享网盘根目录")
		}
	}
	if strings.TrimSpace(params.Password) != "" {
		return nil, errors.New("迅雷分享提取码由服务端生成，暂不支持自定义")
	}
	days, err := shareExpirationDays(params.Expiration)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(params.ShareName)
	if name == "" {
		name = "迅雷云盘分享"
	}
	var result struct {
		ShareID    string `json:"share_id"`
		ShareURL   string `json:"share_url"`
		PassCode   string `json:"pass_code"`
		ShareText  string `json:"share_text"`
		Status     string `json:"share_status"`
		Expiration string `json:"expiration"`
	}
	err = withClient(ctx, account, func(cl *client) error {
		return cl.request(ctx, http.MethodPost, "/share", map[string]any{
			"file_ids": params.FileIDs, "title": name, "share_to": "copy", "restore_limit": "-1", "expiration_days": strconv.Itoa(days),
			"params": map[string]string{"subscribe_push": "false", "WithPassCodeInLink": "true"},
		}, &result)
	})
	if err != nil {
		return nil, err
	}
	shareURL, parseErr := driveutil.ParseShareURL(result.ShareURL, "pan.xunlei.com")
	if result.ShareID == "" || parseErr != nil || shareURL.Scheme != "https" || !strings.HasPrefix(shareURL.Path, "/s/") || strings.TrimPrefix(shareURL.Path, "/s/") == "" {
		return nil, errors.New("迅雷分享响应缺少有效的分享 ID 或官方分享链接，请在官网检查结果")
	}
	if result.Status != "" && result.Status != "OK" {
		return nil, errors.New("迅雷分享尚未通过服务端审核或已失效，请在官网检查结果")
	}
	expiration := result.Expiration
	if expiration == "" && days > 0 {
		expiration = time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	return &model.ShareItem{
		AccountID: account.UserID, DriveID: account.DriveID, ShareID: result.ShareID, ShareURL: result.ShareURL,
		SharePwd: result.PassCode, ShareName: name, SharePolicy: "public", Expiration: expiration,
		FileID: params.FileIDs[0], FileIDList: append([]string(nil), params.FileIDs...), FullShareMsg: result.ShareText,
		Status: result.Status, ShareMsg: "创建成功", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (*Driver) CancelShare(ctx context.Context, account drive.Context, share model.ShareHistoryEntry) error {
	if strings.TrimSpace(share.ShareID) == "" {
		return errors.New("迅雷撤销分享缺少分享 ID")
	}
	if share.Provider != "" && share.Provider != model.ProviderThunder {
		return errors.New("迅雷撤销分享的网盘与当前账号不一致")
	}
	return withClient(ctx, account, func(cl *client) error {
		return cl.mutate(ctx, http.MethodPost, "/share/delete", map[string]string{"space": "", "share_id": share.ShareID})
	})
}
