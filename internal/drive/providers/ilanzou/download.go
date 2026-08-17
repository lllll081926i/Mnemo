package ilanzou

import (
	"context"
	"errors"
	"fmt"
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
	if c.Token == nil {
		return downloadInfo{Error: "未登录"}
	}
	cr := parseCred(c.Token.RefreshToken)
	token := c.Token.AccessToken
	if cr != nil {
		token = firstNonEmpty(token, cr.Token)
	}
	uuid := c.Token.DeviceID
	if cr != nil {
		uuid = firstNonEmpty(uuid, cr.UUID)
	}
	if token == "" || uuid == "" {
		if err := d.reloginDownload(ctx, c); err != nil {
			return downloadInfo{Error: err.Error()}
		}
		cr = parseCred(c.Token.RefreshToken)
		token = c.Token.AccessToken
		uuid = c.Token.DeviceID
		if cr != nil {
			token = firstNonEmpty(token, cr.Token)
			uuid = firstNonEmpty(uuid, cr.UUID)
		}
	}
	if uuid == "" {
		return downloadInfo{Error: "缺少设备 UUID"}
	}
	accountUserID := c.Token.ProviderAccountID
	if cr != nil {
		accountUserID = firstNonEmpty(accountUserID, cr.UserID)
	}
	accountUserID = firstNonEmpty(accountUserID, c.AccountID(), c.UserID)
	info, authFailure := resolveILanzouDownload(ctx, fileID, accountUserID, token, uuid)
	if !authFailure || cr == nil || cr.Username == "" || cr.Password == "" {
		return info
	}
	if err := d.reloginDownload(ctx, c); err != nil {
		return downloadInfo{Error: err.Error()}
	}
	cr = parseCred(c.Token.RefreshToken)
	token = c.Token.AccessToken
	uuid = c.Token.DeviceID
	if cr != nil {
		token = firstNonEmpty(token, cr.Token)
		uuid = firstNonEmpty(uuid, cr.UUID)
		accountUserID = firstNonEmpty(c.Token.ProviderAccountID, cr.UserID)
	}
	accountUserID = firstNonEmpty(accountUserID, c.AccountID(), c.UserID)
	info, _ = resolveILanzouDownload(ctx, fileID, accountUserID, token, uuid)
	return info
}

// reloginDownload rotates the session in place so the drive facade can
// persist it after a download or video-preview request.
func (d *Driver) reloginDownload(ctx context.Context, c drive.Context) error {
	if c.Token == nil {
		return errors.New("未登录")
	}
	cr := parseCred(c.Token.RefreshToken)
	if cr == nil || cr.Username == "" || cr.Password == "" {
		return errors.New("登录凭证已失效，无法自动重登")
	}
	login, err := ilanzouLogin(ctx, cr.Username, cr.Password, cr.UUID)
	if err != nil {
		return fmt.Errorf("优享版蓝奏云自动登录失败: %w", err)
	}
	applyLoginSession(c.Token, login)
	return nil
}

func resolveILanzouDownload(ctx context.Context, fileID, accountUserID, token, uuid string) (downloadInfo, bool) {
	rawURL, err := buildILanzouDownloadUrl(fileID, accountUserID, token, uuid)
	if err != nil {
		return downloadInfo{Error: err.Error()}, false
	}
	u := rawURL
	fetchThrottle.wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return downloadInfo{Error: err.Error()}, false
	}
	req.Header.Set("Referer", ILANZOU_CONF.Site+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := manualClient.Do(req)
	if err != nil {
		return downloadInfo{Error: err.Error()}, false
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
		return downloadInfo{Error: msg}, resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden
	}
	return downloadInfo{
		URL:     u,
		Headers: map[string]string{"Referer": ILANZOU_CONF.Site + "/"},
	}, false
}
