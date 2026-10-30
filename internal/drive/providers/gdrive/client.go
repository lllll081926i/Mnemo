package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	apiHost           = "https://www.googleapis.com"
	apiPath           = "/drive/v3"
	defaultRefreshURL = "https://api.oplist.org/googleui/renewapi"
	folderMIME        = "application/vnd.google-apps.folder"
	shortcutMIME      = "application/vnd.google-apps.shortcut"
	fileFields        = "id,name,mimeType,size,parents,driveId,modifiedTime,createdTime,md5Checksum,sha1Checksum,sha256Checksum,starred,trashed,webViewLink,capabilities(canDownload),shortcutDetails(targetId,targetMimeType,targetResourceKey),resourceKey"
)

type client struct {
	http            *http.Client
	c               drive.Context
	confirming      bool
	credentialError error
	resourceKeys    map[string]string
}

func newClient(c drive.Context) (*client, error) {
	if c.Token == nil || strings.TrimSpace(c.Token.AccessToken) == "" {
		return nil, drive.AuthExpired("Google Drive 未登录，请导入刷新令牌")
	}
	h := netx.NewClientWithSystemProxy(90 * time.Second).HTTP
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client{http: h, c: c, resourceKeys: make(map[string]string)}, nil
}

func refreshEndpoint(raw string) (string, error) {
	if raw == "" {
		raw = defaultRefreshURL
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || !strings.HasSuffix(u.Path, "/googleui/renewapi") || u.RawPath != "" {
		return "", errors.New("OpenList 刷新地址必须是无查询参数的 HTTPS 地址，并以 /googleui/renewapi 结尾")
	}
	return u.String(), nil
}

func safeNetworkError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("%s失败，请检查网络或系统/应用代理", operation)
}

func renew(ctx context.Context, token *model.TokenInfo) error {
	if token == nil || strings.TrimSpace(token.RefreshToken) == "" {
		return drive.AuthExpired("Google Drive 缺少刷新令牌，请重新授权")
	}
	endpoint, err := refreshEndpoint(token.OAuthRefreshURL)
	if err != nil {
		return err
	}
	u, _ := url.Parse(endpoint)
	q := url.Values{"server_use": {"true"}, "driver_txt": {"googleui_go"}, "refresh_ui": {token.RefreshToken}}
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("OpenList 刷新请求构造失败")
	}
	h := netx.NewClientWithSystemProxy(25 * time.Second).HTTP
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := h.Do(req)
	if err != nil {
		return safeNetworkError(ctx, "连接 OpenList 授权服务")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return safeNetworkError(ctx, "读取 OpenList 授权响应")
	}
	var result struct {
		Access      string `json:"access_token"`
		Refresh     string `json:"refresh_token"`
		Text        string `json:"text"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(data, &result) != nil {
		return fmt.Errorf("OpenList 授权服务返回无效响应（HTTP %d）", resp.StatusCode)
	}
	message := strings.ToLower(result.Text + " " + result.Error + " " + result.Description)
	if strings.Contains(message, "deleted_client") || strings.Contains(message, "oauth client was deleted") || strings.Contains(message, "invalid_client") {
		return errors.New("OpenList 的 Google 授权服务当前不可用：共享 OAuth 应用已删除或配置无效，请等待服务恢复，或切换到配置有效应用的自建 OpenList 服务")
	}
	if strings.Contains(message, "invalid_grant") || strings.Contains(message, "expired or revoked") {
		return drive.AuthExpired("Google Drive 刷新令牌已失效或不属于此授权服务，请在同一 OpenList 服务重新授权")
	}
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(result.Access) == "" || result.Text != "" || result.Error != "" {
		return fmt.Errorf("OpenList 未返回有效的 Google 访问令牌（HTTP %d），请检查授权服务和刷新令牌", resp.StatusCode)
	}
	token.AccessToken = result.Access
	if strings.TrimSpace(result.Refresh) != "" {
		token.RefreshToken = result.Refresh
	}
	token.OAuthRefreshURL = endpoint
	token.TokenType = "Bearer"
	token.ExpiresIn = 0
	token.ExpireTime = ""
	return nil
}

func (cl *client) refresh(ctx context.Context, rejected string) error {
	unlock, err := drive.LockTarget(ctx, cl.c.UserID, "", "gdrive-session", "refresh")
	if err != nil {
		return err
	}
	defer unlock()
	if cl.c.UserID != "" {
		if err := drive.ReloadToken(cl.c); err != nil {
			return err
		}
	}
	if rejected != "" && cl.c.Token.AccessToken != rejected {
		if cl.c.TokenSnapshot != nil {
			*cl.c.TokenSnapshot = *drive.CloneToken(cl.c.Token)
		}
		return nil
	}
	if err := renew(ctx, cl.c.Token); err != nil {
		return err
	}
	if cl.c.UserID == "" {
		return nil
	}
	if err := drive.PersistToken(cl.c); err != nil {
		if cl.confirming {
			cl.credentialError = err
			return nil
		}
		return err
	}
	if cl.c.TokenSnapshot != nil {
		*cl.c.TokenSnapshot = *drive.CloneToken(cl.c.Token)
	}
	return nil
}

type apiError struct {
	status int
	reason string
}

func (e *apiError) Error() string {
	switch e.status {
	case 401:
		return "Google Drive 授权未生效，请重新授权"
	case 403:
		switch e.reason {
		case "storageQuotaExceeded":
			return "Google Drive 存储空间不足"
		case "rateLimitExceeded", "userRateLimitExceeded", "dailyLimitExceeded":
			return "Google Drive 请求额度已达上限，请稍后重试"
		case "downloadQuotaExceeded":
			return "Google Drive 此文件的下载额度已达上限，请稍后重试"
		default:
			return "Google Drive 权限不足，无法完成此操作，请检查文件权限或组织策略"
		}
	case 404:
		return "Google Drive 文件不存在或当前账号无权访问"
	case 409:
		return "Google Drive 目标文件已存在，需要核对文件状态"
	case 429:
		return "Google Drive 请求过于频繁，请稍后重试"
	default:
		return fmt.Sprintf("Google Drive 请求失败（HTTP %d）", e.status)
	}
}

func statusError(status int, data []byte) error {
	var raw struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &raw)
	reason := ""
	if len(raw.Error.Errors) > 0 {
		reason = raw.Error.Errors[0].Reason
	}
	return &apiError{status: status, reason: reason}
}

func isStatus(err error, status int) bool {
	var e *apiError
	return errors.As(err, &e) && e.status == status
}

func trustedGoogleURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "www.googleapis.com" && u.User == nil && u.Fragment == "" && (strings.HasPrefix(u.Path, apiPath+"/") || strings.HasPrefix(u.Path, "/upload/drive/v3/"))
}

// exchange 不记录携带令牌的地址；401 只刷新并重放一次，不盲重试写请求。
func (cl *client) exchange(ctx context.Context, method, target string, body []byte, headers http.Header, upload bool) (int, http.Header, []byte, error) {
	if !trustedGoogleURL(target) {
		return 0, nil, nil, errors.New("Google Drive 返回了无效的请求或上传会话地址")
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return 0, nil, nil, errors.New("Google Drive 请求构造失败")
		}
		req.Header = headers.Clone()
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		rejected := cl.c.Token.AccessToken
		req.Header.Set("Authorization", "Bearer "+rejected)
		if len(cl.resourceKeys) > 0 {
			keys := make([]string, 0, len(cl.resourceKeys))
			for id, key := range cl.resourceKeys {
				keys = append(keys, id+"/"+key)
			}
			req.Header.Set("X-Goog-Drive-Resource-Keys", strings.Join(keys, ","))
		}
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		var resp *http.Response
		if upload {
			resp, err = netx.DoUpload(cl.http, req)
		} else {
			resp, err = cl.http.Do(req)
		}
		if err != nil {
			return 0, nil, nil, safeNetworkError(ctx, "Google Drive 请求")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			return resp.StatusCode, resp.Header, nil, safeNetworkError(ctx, "读取 Google Drive 响应")
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			if err := cl.refresh(ctx, rejected); err != nil {
				return resp.StatusCode, resp.Header, nil, err
			}
			continue
		}
		return resp.StatusCode, resp.Header, data, nil
	}
	return 0, nil, nil, errors.New("Google Drive 授权恢复失败")
}

func (cl *client) json(ctx context.Context, method, path string, q url.Values, body, out any) error {
	if q == nil {
		q = make(url.Values)
	}
	if strings.HasPrefix(path, "/files") && path != "/files/generateIds" {
		q.Set("supportsAllDrives", "true")
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	status, _, data, err := cl.exchange(ctx, method, apiHost+apiPath+path+"?"+q.Encode(), payload, nil, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return statusError(status, data)
	}
	if out != nil && json.Unmarshal(data, out) != nil {
		return errors.New("Google Drive 响应格式无效")
	}
	return nil
}

type item struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MIME         string   `json:"mimeType"`
	Size         *int64   `json:"size,string"`
	Parents      []string `json:"parents"`
	DriveID      string   `json:"driveId"`
	Modified     string   `json:"modifiedTime"`
	Created      string   `json:"createdTime"`
	MD5          string   `json:"md5Checksum"`
	SHA1         string   `json:"sha1Checksum"`
	SHA256       string   `json:"sha256Checksum"`
	Starred      bool     `json:"starred"`
	Trashed      bool     `json:"trashed"`
	WebURL       string   `json:"webViewLink"`
	ResourceKey  string   `json:"resourceKey"`
	Capabilities struct {
		CanDownload *bool `json:"canDownload"`
	} `json:"capabilities"`
	Shortcut *struct {
		TargetID    string `json:"targetId"`
		MIME        string `json:"targetMimeType"`
		ResourceKey string `json:"targetResourceKey"`
	} `json:"shortcutDetails"`
}

func (cl *client) detail(ctx context.Context, id string) (*item, error) {
	if !validID(id) {
		return nil, errors.New("Google Drive 文件 ID 无效")
	}
	var result item
	if err := cl.json(ctx, http.MethodGet, "/files/"+id, url.Values{"fields": {fileFields}}, nil, &result); err != nil {
		return nil, err
	}
	if !validID(result.ID) || id != "root" && result.ID != id {
		return nil, errors.New("Google Drive 文件详情缺少有效 ID")
	}
	return &result, nil
}

func validID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func root(c drive.Context, id string) string {
	if id == "" || id == "root" || id == "gdrive_root" {
		if c.Token != nil && c.Token.ProviderRootID != "" {
			return c.Token.ProviderRootID
		}
		return "root"
	}
	return id
}

func quote(s string) string {
	return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s) + "'"
}

func number(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("Google Drive 返回无效的容量或偏移")
	}
	return n, nil
}
