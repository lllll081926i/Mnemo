package ilanzou

import (
	"context"
	"io"
	"net/http"

	"mnemo-go/internal/drive"
)

// downloadInfo is the resolved download result (mirrors the legacy shape).
type downloadInfo struct {
	URL     string
	Size    int64
	Error   string
	Headers map[string]string
}

// downloadInfo resolves the signed /file/redirect URL and follows it to the
// final location (or an error message on 4xx).
func (d *Driver) downloadInfo(ctx context.Context, c drive.Context, fileID string) downloadInfo {
	if c.Token == nil || c.Token.AccessToken == "" {
		return downloadInfo{Error: "未登录"}
	}
	uuid := c.Token.DeviceID
	accountUserID := c.Token.ProviderAccountID
	if cr := parseCred(c.Token.RefreshToken); cr != nil {
		uuid = firstNonEmpty(uuid, cr.UUID)
		accountUserID = firstNonEmpty(accountUserID, cr.UserID)
	}
	if uuid == "" {
		return downloadInfo{Error: "缺少设备 UUID"}
	}
	rawURL, err := buildILanzouDownloadUrl(fileID, firstNonEmpty(accountUserID, c.UserID), c.Token.AccessToken, uuid)
	if err != nil {
		return downloadInfo{Error: err.Error()}
	}
	u := rawURL
	fetchThrottle.wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return downloadInfo{Error: err.Error()}
	}
	req.Header.Set("Referer", ILANZOU_CONF.Site+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := manualClient.Do(req)
	if err != nil {
		return downloadInfo{Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc := resp.Header.Get("Location"); loc != "" {
			u = loc
		}
	} else if resp.StatusCode >= 400 {
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		msg := truncate(string(text), 200)
		if msg == "" {
			msg = "获取下载地址失败"
		}
		return downloadInfo{Error: msg}
	}
	return downloadInfo{
		URL:     u,
		Headers: map[string]string{"Referer": ILANZOU_CONF.Site + "/"},
	}
}
