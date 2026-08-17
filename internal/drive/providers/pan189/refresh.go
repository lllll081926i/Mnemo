package pan189

import (
	"context"
	"encoding/json"
	"strconv"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// RefreshAccount renews the session when near expiry and refreshes the quota
// snapshot from the personal cloud user info. Family accounts query the
// personal-cloud info endpoint (personal signing); quota failures are
// non-blocking (mirrors legacy refreshAccount).
func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, nil
	}
	sess, err := sessionOf(token)
	if err != nil {
		return nil, err
	}
	next, err := d.refreshSession(ctx, token, sess)
	if err != nil {
		return nil, err
	}
	saveSession(token, next)

	// 容量：个人云 getUserInfo（家庭云走个人云查询，失败不阻塞）。
	cc := c
	if cc.Token == nil {
		cc.Token = token
	}
	raw, err := d.request(ctx, cc, apiURL+"/getUserInfo.action", reqOptions{method: "GET", family: boolPtr(false)})
	if err == nil {
		var info struct {
			UserSizeInfo *struct {
				UsedSize          string `json:"usedSize"`
				CloudCapacityInfo *struct {
					TotalSize string `json:"totalSize"`
				} `json:"cloudCapacityInfo"`
			} `json:"userSizeInfo"`
		}
		_ = json.Unmarshal(raw, &info)
		if info.UserSizeInfo != nil {
			if used, err := strconv.ParseInt(info.UserSizeInfo.UsedSize, 10, 64); err == nil && used > 0 {
				token.UsedSize = used
			}
			if total := info.UserSizeInfo.CloudCapacityInfo; total != nil {
				if t, err := strconv.ParseInt(total.TotalSize, 10, 64); err == nil && t > 0 {
					token.TotalSize = t
				}
			}
		}
	}
	return token, nil
}
