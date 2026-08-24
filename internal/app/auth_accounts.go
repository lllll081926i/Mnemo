package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/captcha"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/providers/pan139"
	"mnemo-go/internal/drive/providers/pan189"
	"mnemo-go/internal/drive/providers/pikpak"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

// ClosePikPakCaptcha closes the temporary embedded-challenge callback session.
func (a *App) ClosePikPakCaptcha() {
	logging.Debug("closing PikPak captcha session")
	captcha.Close()
}

func (a *App) startPikPakCaptchaSession() (*captcha.Session, error) {
	logging.Debug("starting PikPak captcha session")
	return captcha.Start(func(session captcha.Session, token string) {
		logging.Info("PikPak captcha callback received", "session_id", session.ID, "has_token", token != "")
		a.emit("pikpak:captcha:completed", map[string]string{
			"session_id":    session.ID,
			"captcha_token": token,
		})
	})
}

// ProviderInfo is the JSON-safe projection of a provider registration exposed
// to the frontend (excludes func-typed Factory/Auth).
type ProviderInfo struct {
	ID           string             `json:"ID"`
	Meta         drive.Meta         `json:"Meta"`
	Capabilities drive.Capabilities `json:"Capabilities"`
	Login        drive.LoginConfig  `json:"Login"`
}

// ---- providers ----

// ListProviders returns the 13 in-scope drive providers (JSON-safe DTOs).
func (a *App) ListProviders() []ProviderInfo {
	regs := drive.All()
	out := make([]ProviderInfo, 0, len(regs))
	for _, r := range regs {
		out = append(out, ProviderInfo{ID: r.ID, Meta: r.Meta, Capabilities: r.Caps, Login: r.Login})
	}
	logging.Debug("providers listed", "count", len(out))
	return out
}

// GetPan189Captcha returns the latest captcha image data URL for 189 cloud.
func (a *App) GetPan189Captcha() string {
	return pan189.CaptchaImage()
}

// SendPan139SMS sends the second-factor code for a pending 139 password login.
func (a *App) SendPan139SMS(username string) error {
	started := logActionStarted("发送139短信验证码", "login", "", "", "provider", model.ProviderPan139)
	err := pan139.RequestPan139SMS(a.appContext(), username)
	logActionFinished("发送139短信验证码", "login", "", "", started, err, "provider", model.ProviderPan139)
	return err
}

// SendPan189SMS starts a Tianyi Cloud SMS login and retains the login session
// for the subsequent ProviderLogin call carrying the verification code.
func (a *App) SendPan189SMS(username string) error {
	started := logActionStarted("发送天翼云盘短信验证码", "login", "", "", "provider", model.ProviderPan189)
	err := pan189.RequestPan189SMS(a.appContext(), username)
	logActionFinished("发送天翼云盘短信验证码", "login", "", "", started, err, "provider", model.ProviderPan189)
	return err
}

// ProviderLogin performs a login for a provider with form config.
func (a *App) ProviderLogin(provider string, config map[string]string) (*model.Account, error) {
	started := time.Now()
	logging.Info("provider login started", "provider", provider, "config_keys", configKeys(config), "has_captcha_token", strings.TrimSpace(config["captcha_token"]) != "")
	reg, ok := drive.Get(provider)
	if !ok {
		logging.Error("provider login rejected", "provider", provider, "reason", "unknown provider")
		return nil, fmt.Errorf("未知网盘: %s", provider)
	}
	if reg.Auth == nil {
		logging.Error("provider login rejected", "provider", provider, "reason", "auth unsupported")
		return nil, fmt.Errorf("%s 不支持此登录方式", provider)
	}
	// inject secrets
	if config == nil {
		config = map[string]string{}
	}
	secrets := a.secretsSnapshot()
	if secrets.OnedriveClientID != "" {
		config["onedrive_client_id"] = secrets.OnedriveClientID
	}
	if secrets.DropboxAppKey != "" {
		config["dropbox_app_key"] = secrets.DropboxAppKey
	}
	if secrets.DropboxRedirectURI != "" {
		config["dropbox_redirect_uri"] = secrets.DropboxRedirectURI
	}
	tok, err := reg.Auth(a.appContext(), drive.AuthRequest{
		Config: config,
		Open: func(url string) error {
			ctx, ok := a.wailsContext()
			if !ok {
				return errors.New("应用尚未启动，无法打开登录页面")
			}
			runtime.BrowserOpenURL(ctx, url)
			return nil
		},
	})
	if err != nil {
		var challenge *pikpak.CaptchaRequiredError
		if provider == model.ProviderPikpak && errors.As(err, &challenge) {
			// Keep the rclone request payload untouched. A callback session is only
			// needed after PikPak actually returns a visual challenge, and the
			// frontend embeds that challenge in the existing login page.
			session, sessionErr := a.startPikPakCaptchaSession()
			if sessionErr != nil {
				logging.Error("PikPak captcha session initialization failed", "error", sessionErr)
				return nil, fmt.Errorf("PikPak 验证会话初始化失败: %w", sessionErr)
			}
			logging.Info("provider login challenge required", "provider", provider, "session_id", session.ID)
			return nil, fmt.Errorf("%w\nsession=%s\ncallback=%s", err, session.ID, session.CallbackURL)
		}
		logging.Warn("provider login failed", "provider", provider, "error", err, "duration", logging.Duration(started))
		return nil, err
	}
	if tok == nil {
		logging.Error("provider login returned empty session", "provider", provider)
		return nil, fmt.Errorf("%s 登录未返回会话", provider)
	}
	// The registration that performed the login is authoritative. Do not let a
	// stale provider marker from an imported token route later requests to a
	// different driver.
	tok.TokenFrom = provider
	// Every provider must return a stable account identity. Normalize the
	// namespace here so a provider-specific login cannot create an empty or
	// cross-provider account key.
	accountID := model.StripUserID(provider, tok.UserID)
	if accountID == "" {
		logging.Error("provider login returned no account identity", "provider", provider)
		accountID = strings.TrimSpace(tok.ProviderAccountID)
	}
	if accountID == "" {
		return nil, fmt.Errorf("%s 登录成功但未返回账号标识", provider)
	}
	tok.ProviderAccountID = accountID
	tok.UserID = model.BuildUserID(provider, accountID)
	if tok.DefaultDriveID == "" {
		tok.DefaultDriveID = model.BuildDriveID(provider, accountID)
	}
	acc := &model.Account{
		UserID:  tok.UserID,
		DriveID: normalizedDriveID(provider, accountID, tok.DefaultDriveID),
		Token:   tok,
	}
	if acc.DriveID == "" {
		acc.DriveID = model.BuildDriveID(provider, tok.ProviderAccountID)
	}
	syncAccountUsage(acc)
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveAccount(acc); err != nil {
		logging.Error("provider account persistence failed", "provider", provider, "error", err)
		return nil, err
	}
	a.markAccountRefreshSuccess(acc.UserID)
	logging.Info("provider login completed", "provider", provider, "account_id", redactID(accountID), "duration", logging.Duration(started))
	a.emit("account:changed", acc)
	return acc, nil
}

// SaveMountedAccount persists a mounted storage account (webdav/s3) from the
// connection form.
func (a *App) SaveMountedAccount(provider string, conn model.ConnConfig) (*model.Account, error) {
	started := time.Now()
	logging.Info("mounted account save started", "provider", provider, "endpoint_host", urlHost(conn.Endpoint))
	if provider != model.ProviderWebdav && provider != model.ProviderS3 {
		logging.Error("mounted account save rejected", "provider", provider, "reason", "unsupported provider")
		return nil, fmt.Errorf("仅支持挂载存储: %s", provider)
	}
	if err := drive.ValidateConnection(provider, &conn); err != nil {
		logging.Warn("mounted account validation failed", "provider", provider, "error", err)
		return nil, fmt.Errorf("连接校验失败: %w", err)
	}
	accountID := mountedAccountID(provider, conn)
	uid := model.BuildUserID(provider, accountID)
	tok := &model.TokenInfo{
		TokenFrom: provider,
		UserID:    uid,
		UserName:  conn.Name,
		Conn:      &conn,
	}
	acc := &model.Account{
		UserID:  uid,
		DriveID: model.BuildDriveID(provider, accountID),
		Token:   tok,
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveAccount(acc); err != nil {
		logging.Error("mounted account persistence failed", "provider", provider, "error", err)
		return nil, err
	}
	logging.Info("mounted account save completed", "provider", provider, "account_id", redactID(accountID), "duration", logging.Duration(started))
	a.emit("account:changed", acc)
	return acc, nil
}

// RenameMountedAccount changes only the local display name of a WebDAV/S3
// mount. Its stable user/drive ids and encrypted connection credentials are
// intentionally preserved.
func (a *App) RenameMountedAccount(userID, name string) (*model.Account, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("网盘名称不能为空")
	}
	if len([]rune(name)) > 80 {
		return nil, fmt.Errorf("网盘名称不能超过 80 个字符")
	}
	for _, r := range name {
		if r == '\r' || r == '\n' || r == '\t' {
			return nil, fmt.Errorf("网盘名称不能包含换行或制表符")
		}
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	acc, err := st.GetAccount(strings.TrimSpace(userID))
	if err != nil || acc == nil {
		return nil, fmt.Errorf("账号不存在")
	}
	provider := acc.Provider()
	if provider != model.ProviderWebdav && provider != model.ProviderS3 {
		return nil, fmt.Errorf("仅支持重命名 WebDAV/S3 挂载账号")
	}
	updated, err := st.RenameMountedAccount(acc.UserID, name)
	if err != nil {
		logging.Warn("mounted account rename failed", "provider", provider, "account_id", redactID(acc.UserID), "error", err)
		return nil, err
	}
	a.emit("account:changed", updated)
	logging.Info("mounted account renamed", "provider", provider, "account_id", redactID(acc.UserID))
	return updated, nil
}

// SetAccountCustomMeta saves custom display name and custom icon for any cloud account.
func (a *App) SetAccountCustomMeta(userID, customName, customIcon string) (*model.Account, error) {
	started := logActionStarted("保存账号自定义", "account", userID, "",
		"has_name", strings.TrimSpace(customName) != "", "has_icon", strings.TrimSpace(customIcon) != "")
	st, err := a.storeOrError()
	if err != nil {
		logActionFinished("保存账号自定义", "account", userID, "", started, err)
		return nil, err
	}
	updated, err := st.UpdateAccountCustomMeta(userID, customName, customIcon)
	if err != nil {
		logActionFinished("保存账号自定义", "account", userID, "", started, err)
		return nil, err
	}
	a.emit("account:changed", updated)
	logActionFinished("保存账号自定义", "account", userID, "", started, nil,
		"has_name", updated.CustomName != "", "has_icon", updated.CustomIcon != "")
	return updated, nil
}

// ValidateMountedWrite performs an explicitly requested S3 write probe. It
// does not persist an account or run during the normal login check.
func (a *App) ValidateMountedWrite(provider string, conn model.ConnConfig) error {
	if provider != model.ProviderS3 {
		return fmt.Errorf("当前网盘不支持可选写入验证")
	}
	if err := drive.ValidateWriteConnection(provider, &conn); err != nil {
		logging.Warn("mounted write validation failed", "provider", provider, "endpoint_host", urlHost(conn.Endpoint), "error", err)
		return err
	}
	logging.Info("mounted write validation completed", "provider", provider, "endpoint_host", urlHost(conn.Endpoint))
	return nil
}

// mountedAccountID namespaces mounted connections by their non-secret
// connection identity. Endpoint alone is insufficient when one server has
// multiple users, and a display name alone can collide as well.
func mountedAccountID(provider string, conn model.ConnConfig) string {
	pathKey := conn.BasePath
	if provider == model.ProviderWebdav {
		if strings.TrimSpace(conn.RootPath) != "" {
			pathKey = conn.RootPath
		}
	}
	identity := strings.Join([]string{
		provider,
		strings.TrimSpace(conn.Name),
		strings.TrimSpace(conn.Endpoint),
		strings.TrimSpace(conn.Username),
		strings.ToLower(strings.TrimSpace(conn.AuthType)),
		strings.TrimSpace(conn.Region),
		strings.TrimSpace(conn.Bucket),
		strings.TrimSpace(pathKey),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "mount-" + hex.EncodeToString(sum[:12])
}

// normalizedDriveID keeps provider-returned drive identities (notably the
// real Microsoft Graph drive id) while retaining the provider namespace used
// by the application and cache keys.
func normalizedDriveID(provider, accountID, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		if strings.HasPrefix(preferred, provider+":") {
			return preferred
		}
		return model.BuildDriveID(provider, preferred)
	}
	return model.BuildDriveID(provider, accountID)
}
