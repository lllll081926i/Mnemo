package guangya

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const guangyaCreateSharePath = "/nd.bizuserres.s/v1/share_file"

// CreateShare creates a public Guangya share link through the same resource
// API used by the official web client. Guangya accepts multiple file IDs in
// one share, an optional extraction code, and a duration in whole days.
func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	fileIDs := normalizeGuangyaShareIDs(params.FileIDs)
	if len(fileIDs) == 0 {
		return nil, errors.New("光鸭云盘创建分享至少选择一个文件或文件夹")
	}
	duration, err := guangyaShareDuration(params.Expiration)
	if err != nil {
		return nil, err
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(params.ShareName)
	if title == "" {
		title = "光鸭云盘分享"
	}
	body := map[string]any{
		"fileIds":          fileIDs,
		"title":            title,
		"validateDuration": duration,
		"shareType":        1,
		"code":             strings.TrimSpace(params.Password),
		"autoFillCode":     true,
		"trafficLimit":     "0",
		"maxRestoreCount":  0,
		"downloadType":     1,
	}
	var response map[string]any
	if err := cl.post(ctx, guangyaCreateSharePath, body, &response); err != nil {
		return nil, err
	}
	data, err := guangyaShareResponseData(response)
	if err != nil {
		return nil, err
	}
	shareURL := guangyaShareField(data, response, "shareUrl", "shareURL", "shareLink", "share_link", "url", "link")
	if shareURL == "" {
		return nil, errors.New("光鸭云盘创建分享未返回链接")
	}
	shareID := guangyaShareField(data, response, "shareId", "shareID", "share_id", "id")
	if shareID == "" {
		shareID = shareURL
	}
	password := guangyaShareField(data, response, "shareCode", "sharePwd", "sharePassword", "password", "pwd", "passCode", "passcode", "code")
	if password == "" {
		password = strings.TrimSpace(params.Password)
	}
	return &model.ShareItem{
		AccountID:   c.UserID,
		DriveID:     c.DriveID,
		ShareID:     shareID,
		ShareURL:    shareURL,
		SharePwd:    password,
		ShareName:   title,
		SharePolicy: "public",
		Expiration:  params.Expiration,
		FileID:      fileIDs[0],
		FileIDList:  fileIDs,
		ShareMsg:    "创建成功",
	}, nil
}

func normalizeGuangyaShareIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func guangyaShareDuration(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	if days, err := strconv.Atoi(value); err == nil {
		if days < 0 || days > 36500 {
			return 0, errors.New("光鸭云盘分享有效期需在 1 天至 100 年之间，或选择永久有效")
		}
		return days, nil
	}
	target, err := parseGuangyaShareTime(value)
	if err != nil {
		return 0, errors.New("光鸭云盘分享有效期格式无效")
	}
	remaining := time.Until(target)
	if remaining <= 0 {
		return 0, errors.New("光鸭云盘分享有效期已过期")
	}
	days := int(math.Ceil(remaining.Hours() / 24))
	if days < 1 || days > 36500 {
		return 0, errors.New("光鸭云盘分享有效期需在 1 天至 100 年之间，或选择永久有效")
	}
	return days, nil
}

func parseGuangyaShareTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid time")
}

func guangyaShareResponseData(response map[string]any) (map[string]any, error) {
	if len(response) == 0 {
		return nil, errors.New("光鸭云盘创建分享响应为空")
	}
	if code, exists := response["code"]; exists && !guangyaShareSuccessCode(code) {
		message := guangyaShareField(response, nil, "message", "msg", "error_description", "error")
		if message == "" {
			message = "服务端未说明原因"
		}
		return nil, fmt.Errorf("光鸭云盘创建分享失败: %s", message)
	}
	for _, key := range []string{"data", "result", "shareInfo", "share", "shareFile"} {
		if data, ok := response[key].(map[string]any); ok && len(data) > 0 {
			return data, nil
		}
	}
	return response, nil
}

func guangyaShareSuccessCode(value any) bool {
	switch code := value.(type) {
	case nil:
		return true
	case float64:
		return code == 0 || code == 200
	case float32:
		return code == 0 || code == 200
	case int:
		return code == 0 || code == 200
	case int64:
		return code == 0 || code == 200
	case string:
		code = strings.TrimSpace(strings.ToLower(code))
		return code == "" || code == "0" || code == "200" || code == "ok" || code == "success"
	case bool:
		return code
	default:
		return false
	}
}

func guangyaShareField(primary, fallback map[string]any, keys ...string) string {
	for _, source := range []map[string]any{primary, fallback} {
		for _, key := range keys {
			if source == nil {
				continue
			}
			if value := strings.TrimSpace(str(source[key])); value != "" {
				return value
			}
		}
	}
	return ""
}
