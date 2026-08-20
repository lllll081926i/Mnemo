package pan189

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// RefreshAccount renews the session and refreshes the personal-cloud quota.
// 天翼家庭云的容量与个人云独立，现有已确认接口不能返回家庭容量；因此
// 不以个人云容量冒充家庭云容量。
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
	if isFamily, _ := cloudInfo(next); isFamily {
		token.TotalSize = 0
		token.UsedSize = 0
		token.FreeSize = 0
		return token, nil
	}

	// 容量：个人云 getUserInfo。容量接口失败不影响日常文件操作，也不
	// 清空上一次成功缓存，避免瞬时错误导致容量闪烁。
	cc := c
	if cc.Token == nil {
		cc.Token = token
	}
	raw, err := d.request(ctx, cc, apiURL+"/getUserInfo.action", reqOptions{method: "GET", family: boolPtr(false)})
	if err == nil {
		usedSize, totalSize, ok := parsePan189Quota(raw)
		applyPan189Quota(token, usedSize, totalSize, ok)
	}
	return token, nil
}

func parsePan189Quota(raw []byte) (usedSize, totalSize int64, ok bool) {
	var response struct {
		UserSizeInfo struct {
			UsedSize          json.RawMessage `json:"usedSize"`
			CloudCapacityInfo struct {
				TotalSize json.RawMessage `json:"totalSize"`
			} `json:"cloudCapacityInfo"`
		} `json:"userSizeInfo"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return 0, 0, false
	}
	usedSize, usedOK := pan189QuotaInt64(response.UserSizeInfo.UsedSize)
	totalSize, totalOK := pan189QuotaInt64(response.UserSizeInfo.CloudCapacityInfo.TotalSize)
	if !usedOK || !totalOK || totalSize <= 0 {
		return 0, 0, false
	}
	if usedSize > totalSize {
		usedSize = totalSize
	}
	return usedSize, totalSize, true
}

func pan189QuotaInt64(raw json.RawMessage) (int64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false
	}
	var stringValue string
	if json.Unmarshal(raw, &stringValue) == nil {
		text = strings.TrimSpace(stringValue)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil && value >= 0
}

func applyPan189Quota(token *model.TokenInfo, usedSize, totalSize int64, ok bool) {
	if token == nil || !ok || totalSize <= 0 || usedSize < 0 {
		return
	}
	if usedSize > totalSize {
		usedSize = totalSize
	}
	token.TotalSize = totalSize
	token.UsedSize = usedSize
	token.FreeSize = totalSize - usedSize
}
