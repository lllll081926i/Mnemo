package aliopen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// Session is the raw credential JSON blob.
type Session struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	DriveID          string `json:"drive_id"`
	ResourceDriveID  string `json:"resource_drive_id"`
	BackupDriveID    string `json:"backup_drive_id"`
	UserID           string `json:"user_id,omitempty"`
	UserName         string `json:"user_name,omitempty"`
	NickName         string `json:"nick_name,omitempty"`
	Phone            string `json:"phone,omitempty"`
	Avatar           string `json:"avatar,omitempty"`
	ProfileCheckedAt int64  `json:"profile_checked_at,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	ClientSecret     string `json:"client_secret,omitempty"`
	OAuthTokenURL    string `json:"oauth_token_url,omitempty"`
}

// aliOpenFlexString accepts identifiers returned by the Open API as either
// JSON strings or JSON numbers. Default drive IDs are numeric for some apps.
type aliOpenFlexString string

func (s *aliOpenFlexString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*s = aliOpenFlexString(value)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*s = aliOpenFlexString(number.String())
	return nil
}

func (s aliOpenFlexString) String() string { return string(s) }

type aliOpenTokenResponse struct {
	AccessToken     string            `json:"access_token"`
	RefreshToken    string            `json:"refresh_token"`
	UserID          string            `json:"user_id"`
	UserName        string            `json:"user_name"`
	NickName        string            `json:"nick_name"`
	Phone           string            `json:"phone"`
	Mobile          string            `json:"mobile"`
	Avatar          string            `json:"avatar"`
	DefaultDriveID  aliOpenFlexString `json:"default_drive_id"`
	ResourceDriveID aliOpenFlexString `json:"resource_drive_id"`
	BackupDriveID   aliOpenFlexString `json:"backup_drive_id"`
}

// aliOpenAccountProfile is returned by the consumer profile endpoint. Open
// token responses commonly contain only identifiers, while this endpoint
// exposes the display name / masked phone that should be shown to the user.
type aliOpenAccountProfile struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	NickName string `json:"nick_name"`
	Phone    string `json:"phone"`
	Mobile   string `json:"mobile"`
	Avatar   string `json:"avatar"`
}

func (s *Session) applyTokenProfile(res aliOpenTokenResponse) {
	if s == nil {
		return
	}
	if value := strings.TrimSpace(res.UserID); value != "" {
		s.UserID = value
	}
	if value := strings.TrimSpace(res.UserName); value != "" {
		s.UserName = value
	}
	if value := strings.TrimSpace(res.NickName); value != "" {
		s.NickName = value
	}
	if value := firstAliOpenProfileValue(res.Phone, res.Mobile); value != "" {
		s.Phone = value
	}
	if value := strings.TrimSpace(res.Avatar); value != "" {
		s.Avatar = value
	}
	// Keep an already resolved drive route stable. A fresh session receives its
	// default drive directly from the token response and avoids an extra call.
	if s.DriveID == "" {
		s.DriveID = strings.TrimSpace(res.DefaultDriveID.String())
	}
	if s.ResourceDriveID == "" {
		s.ResourceDriveID = strings.TrimSpace(res.ResourceDriveID.String())
	}
	if s.BackupDriveID == "" {
		s.BackupDriveID = strings.TrimSpace(res.BackupDriveID.String())
	}
}

func (s *Session) applyAccountProfile(profile aliOpenAccountProfile) {
	if s == nil {
		return
	}
	if value := strings.TrimSpace(profile.UserID); value != "" {
		s.UserID = value
	}
	if value := strings.TrimSpace(profile.UserName); value != "" {
		s.UserName = value
	}
	if value := strings.TrimSpace(profile.NickName); value != "" {
		s.NickName = value
	}
	if value := firstAliOpenProfileValue(profile.Phone, profile.Mobile); value != "" {
		s.Phone = value
	}
	if value := strings.TrimSpace(profile.Avatar); value != "" {
		s.Avatar = value
	}
}

func (s *Session) accountID() string {
	if s == nil {
		return ""
	}
	for _, value := range []string{s.UserID, s.DriveID} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (s *Session) displayName() string {
	if s == nil {
		return ""
	}
	if value := s.profileName(); value != "" {
		return value
	}
	return s.accountID()
}

// profileName returns only a human-facing value. The Open token's user_id and
// drive_id are stable identifiers, but must not keep winning over a later
// nickname/phone refresh in the account UI.
func (s *Session) profileName() string {
	if s == nil {
		return ""
	}
	for _, value := range []string{s.NickName, s.UserName, s.Phone} {
		if value = s.profileValue(value); value != "" {
			return value
		}
	}
	return ""
}

func (s *Session) profileValue(value string) string {
	if s == nil {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" || value == strings.TrimSpace(s.UserID) || value == strings.TrimSpace(s.DriveID) {
		return ""
	}
	return value
}

func (s *Session) profileRefreshDue(now time.Time) bool {
	if s == nil || s.ProfileCheckedAt <= 0 {
		return s != nil
	}
	interval := aliOpenProfileRetryInterval
	if s.profileName() != "" {
		interval = aliOpenProfileSuccessInterval
	}
	return now.Sub(time.Unix(s.ProfileCheckedAt, 0)) >= interval
}

func applyAliOpenProfile(token *model.TokenInfo, sess *Session) {
	if token == nil || sess == nil {
		return
	}
	if value := strings.TrimSpace(sess.Avatar); value != "" {
		token.Avatar = value
	}
	name := sess.profileName()
	if name == "" {
		return
	}
	// Keep all three display fields coherent: the frontend prefers nick_name,
	// so an old identifier left there would otherwise hide a newly fetched name.
	userName := sess.profileValue(sess.UserName)
	nickName := sess.profileValue(sess.NickName)
	phone := sess.profileValue(sess.Phone)
	token.UserName = firstAliOpenProfileValue(userName, phone, name)
	token.NickName = firstAliOpenProfileValue(nickName, userName, phone, name)
	token.Name = name
}

func applyAliOpenQuota(token *model.TokenInfo, used, total int64) {
	if token == nil || total <= 0 {
		return
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}
	token.UsedSize = used
	token.TotalSize = total
	token.FreeSize = total - used
}

// GetSpaceInfo returns account quota.
func (c *client) GetSpaceInfo(ctx context.Context) (used, total int64) {
	var raw json.RawMessage
	if err := c.apiPost(ctx, "/adrive/v1.0/user/getSpaceInfo", map[string]any{}, &raw); err != nil {
		return 0, 0
	}
	return parseAliOpenSpaceInfo(raw)
}

func parseAliOpenSpaceInfo(raw json.RawMessage) (used, total int64) {
	var response struct {
		PersonalSpaceInfo map[string]json.RawMessage `json:"personal_space_info"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return 0, 0
	}
	used, usedOK := aliOpenInt64(response.PersonalSpaceInfo, "used_size", "usedSize", "used")
	total, totalOK := aliOpenInt64(response.PersonalSpaceInfo, "total_size", "totalSize", "total")
	if !usedOK || !totalOK || total <= 0 {
		return 0, 0
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}
	return used, total
}

func aliOpenInt64(values map[string]json.RawMessage, keys ...string) (int64, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || string(raw) == "null" {
			continue
		}
		var value aliOpenFlexString
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(value.String()), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func clientOf(c drive.Context) (*client, error) {
	if c.Token == nil {
		return nil, drive.ErrUnauthorized
	}
	sess := parseSession(c.Token)
	if sess == nil {
		return nil, errors.New("aliopen: invalid session")
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess, token: c.Token}
	if sess.AccessToken == "" {
		if err := cl.refresh(context.Background(), c.UserID); err != nil {
			return nil, err
		}
	}
	if sess.DriveID == "" {
		if err := cl.ensureDrive(context.Background()); err != nil {
			return nil, err
		}
	}
	cl.persistSession()
	return cl, nil
}

func parseSession(tok *model.TokenInfo) *Session {
	if tok == nil {
		return nil
	}
	var sess Session
	raw := tok.RefreshToken
	if raw == "" && tok.OpenAPIRefreshToken != "" {
		raw = tok.OpenAPIRefreshToken
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &sess); err == nil && (sess.RefreshToken != "" || sess.AccessToken != "") {
			if sess.AccessToken == "" && tok.AccessToken != "" {
				sess.AccessToken = tok.AccessToken
			}
			if sess.RefreshToken == "" && tok.OpenAPIRefreshToken != "" {
				sess.RefreshToken = tok.OpenAPIRefreshToken
			}
			return &sess
		}
		// Older builds persisted the refresh token itself instead of the JSON
		// session. Keep those accounts refreshable after migration.
		if len(strings.TrimSpace(raw)) > 20 {
			return &Session{AccessToken: tok.AccessToken, RefreshToken: strings.TrimSpace(raw)}
		}
	}
	if tok.AccessToken != "" {
		return &Session{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}
	}
	return nil
}

func (c *client) persistSession() {
	if c == nil || c.token == nil || c.session == nil {
		return
	}
	c.token.AccessToken = c.session.AccessToken
	c.token.RefreshToken = mustJSON(c.session)
	c.token.OpenAPIAccessToken = c.session.AccessToken
	c.token.OpenAPIRefreshToken = c.session.RefreshToken
}

func (c *client) refresh(ctx context.Context, _ string) error {
	if err := c.refreshToken(ctx); err != nil {
		return err
	}
	if c.session.DriveID == "" {
		if err := c.ensureDrive(ctx); err != nil {
			return err
		}
	}
	c.persistSession()
	return nil
}

func (c *client) refreshToken(ctx context.Context) error {
	if strings.TrimSpace(c.session.RefreshToken) == "" {
		return errors.New("aliopen: refresh_token 缺失")
	}
	url := c.session.OAuthTokenURL
	if c.session.ClientID != "" {
		url = apiHost + "/oauth/access_token"
	}
	if url == "" {
		url = oauthDefault
	}
	body := map[string]string{"grant_type": "refresh_token", "refresh_token": c.session.RefreshToken}
	if c.session.ClientID != "" {
		body["client_id"] = c.session.ClientID
		if c.session.ClientSecret != "" {
			body["client_secret"] = c.session.ClientSecret
		}
	}
	var resp *http.Response
	if err := aliOpenLimiter.run(ctx, func() error {
		var err error
		resp, err = c.http.Do(ctx, http.MethodPost, url, map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		}, netx.JSONBody(body))
		return err
	}); err != nil {
		return err
	}
	data, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		aliOpenLimiter.penalize(aliOpenRetryAfter(resp, 8*time.Second))
	}
	if resp.StatusCode >= 400 {
		return aliOpenRequestErrorOf(data, resp.StatusCode)
	}
	var res aliOpenTokenResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("aliopen: refresh response: %w", err)
	}
	if res.AccessToken == "" {
		return aliOpenRequestErrorOf(data, resp.StatusCode)
	}
	c.session.AccessToken = res.AccessToken
	if res.RefreshToken != "" {
		c.session.RefreshToken = res.RefreshToken
	}
	c.session.applyTokenProfile(res)
	c.persistSession()
	return nil
}

// refreshAccountProfile obtains display metadata only when it is due. It is
// deliberately best-effort: a provider's profile endpoint must not make an
// otherwise healthy token or quota refresh fail. The persisted timestamp keeps
// an unavailable endpoint from being retried on every application launch.
func (c *client) refreshAccountProfile(ctx context.Context) {
	if c == nil || c.http == nil || c.session == nil || !c.session.profileRefreshDue(time.Now()) {
		return
	}
	c.session.ProfileCheckedAt = time.Now().Unix()

	var resp *http.Response
	err := aliOpenLimiter.run(ctx, func() error {
		var requestErr error
		resp, requestErr = c.http.Do(ctx, http.MethodPost, profileAPIHost+"/v2/user/get", map[string]string{
			"Authorization": "Bearer " + c.session.AccessToken,
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Origin":        "https://www.alipan.com",
			"Referer":       "https://www.alipan.com/",
		}, netx.JSONBody(map[string]any{}))
		return requestErr
	})
	if err != nil || resp == nil {
		return
	}
	data, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		aliOpenLimiter.penalize(aliOpenRetryAfter(resp, 8*time.Second))
	}
	if readErr != nil || resp.StatusCode >= http.StatusBadRequest {
		return
	}
	var profile aliOpenAccountProfile
	if json.Unmarshal(data, &profile) != nil {
		return
	}
	c.session.applyAccountProfile(profile)
}

func (c *client) ensureDrive(ctx context.Context) error {
	type driveInfo struct {
		DefaultDriveID  aliOpenFlexString `json:"default_drive_id"`
		ResourceDriveID aliOpenFlexString `json:"resource_drive_id"`
		BackupDriveID   aliOpenFlexString `json:"backup_drive_id"`
	}
	var info driveInfo
	if err := c.apiPost(ctx, "/adrive/v1.0/user/getDriveInfo", map[string]any{}, &info); err != nil {
		return err
	}
	driveID := strings.TrimSpace(info.DefaultDriveID.String())
	if driveID == "" {
		driveID = strings.TrimSpace(info.ResourceDriveID.String())
	}
	if driveID == "" {
		driveID = strings.TrimSpace(info.BackupDriveID.String())
	}
	if driveID == "" {
		return errors.New("aliopen: 获取 drive_id 失败")
	}
	c.session.DriveID = driveID
	c.session.ResourceDriveID = strings.TrimSpace(info.ResourceDriveID.String())
	c.session.BackupDriveID = strings.TrimSpace(info.BackupDriveID.String())
	c.persistSession()
	return nil
}

func (c *client) scopedDriveID(scope Scope) string {
	if scope == ScopeResource && c.session.ResourceDriveID != "" {
		return c.session.ResourceDriveID
	}
	return c.session.DriveID
}

// apiPost calls the Aliyun API with auth and rate limiting.

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	sess := parseSession(token)
	if sess == nil {
		return nil, errors.New("aliopen: 会话不存在，请重新登录")
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess, token: token}
	if err := cl.refresh(ctx, c.UserID); err != nil {
		return nil, err
	}
	used, total := cl.GetSpaceInfo(ctx)
	// Avoid a separate profile request during a routine quota refresh. Account
	// identity comes from the refresh-token response, while the refresh path
	// stays low-frequency and predictable for provider risk control.
	cl.persistSession()
	applyAliOpenProfile(token, sess)
	applyAliOpenQuota(token, used, total)
	return token, nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// authRefreshToken handles aliyun open login with a refresh_token.
func authRefreshToken(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	refreshToken := strings.TrimSpace(req.Config["refresh_token"])
	if refreshToken == "" {
		return nil, errors.New("aliopen: 请输入 refresh_token")
	}
	clientID := strings.TrimSpace(req.Config["client_id"])
	clientSecret := strings.TrimSpace(req.Config["client_secret"])

	sess := &Session{
		RefreshToken: refreshToken,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess}
	if err := cl.refresh(ctx, ""); err != nil {
		return nil, err
	}
	used, total := cl.GetSpaceInfo(ctx)
	uid := sess.accountID()
	if uid == "" {
		return nil, errors.New("aliopen: 登录成功但未返回账号标识")
	}
	name := sess.displayName()
	if name == "" {
		name = uid
	}

	tok := &model.TokenInfo{
		TokenFrom:           providerID,
		AccessToken:         sess.AccessToken,
		RefreshToken:        mustJSON(sess),
		OpenAPIAccessToken:  sess.AccessToken,
		OpenAPIRefreshToken: sess.RefreshToken,
		TokenType:           "Bearer",
		UserID:              model.BuildUserID(providerID, uid),
		UserName:            firstAliOpenProfileValue(sess.UserName, sess.NickName, sess.Phone, name),
		NickName:            firstAliOpenProfileValue(sess.NickName, sess.UserName, sess.Phone, name),
		Name:                name,
		DefaultDriveID:      model.BuildDriveID(providerID, uid),
		ProviderAccountID:   uid,
		ProviderRootID:      "root",
		DeviceID:            "mnemo",
	}
	if sess.Avatar != "" {
		tok.Avatar = sess.Avatar
	}
	applyAliOpenQuota(tok, used, total)
	return tok, nil
}

func firstAliOpenProfileValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
