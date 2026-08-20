package lanzou

import (
	"context"
	"errors"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// authLogin is the registration AuthFunc (port of legacy lanzou auth.ts):
// it accepts a persisted Cookie directly, or username+password.
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	tier := normalizeLanzouUploadTier(req.Config["upload_tier"])
	cookie := strings.TrimSpace(req.Config["cookie"])
	if cookie != "" {
		token, err := loginLanzouWithCookie(ctx, cookie)
		if err == nil {
			setLanzouUploadTier(token, tier)
		}
		return token, err
	}
	account := strings.TrimSpace(req.Config["username"])
	password := req.Config["password"]
	if account == "" || password == "" {
		return nil, errors.New("请输入蓝奏云 Cookie 或账号密码")
	}
	token, err := loginLanzouWithAccount(ctx, account, password)
	if err == nil {
		setLanzouUploadTier(token, tier)
	}
	return token, err
}

// loginLanzouWithCookie validates a Cookie and builds the session token.
func loginLanzouWithCookie(ctx context.Context, cookie string) (*model.TokenInfo, error) {
	ck := strings.TrimSpace(cookie)
	if ck == "" {
		return nil, errors.New("请输入蓝奏云 Cookie")
	}
	uid, vei := lanzouGetVeiAndUid(ctx, ck, LANZOU_DEFAULT.BaseURL)
	if uid == "" {
		return nil, errors.New("无法从 Cookie 解析用户信息，请确认 Cookie 有效")
	}
	return buildLanzouToken("cookie", ck, uid, "", vei, "", ""), nil
}

// loginLanzouWithAccount logs in with credentials and persists them for refresh.
func loginLanzouWithAccount(ctx context.Context, account, password string) (*model.TokenInfo, error) {
	user := strings.TrimSpace(account)
	pass := password
	if user == "" || pass == "" {
		return nil, errors.New("请输入蓝奏账号和密码")
	}
	cookie, err := lanzouAccountLogin(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	uid, vei := lanzouGetVeiAndUid(ctx, cookie, LANZOU_DEFAULT.BaseURL)
	id := uid
	if id == "" {
		id = user
	}
	return buildLanzouToken("account", cookie, id, user, vei, user, pass), nil
}

// buildLanzouToken assembles the TokenInfo (annotations mirror the legacy).
func buildLanzouToken(kind, cookie, uid, displayName, vei, account, password string) *model.TokenInfo {
	refresh := mustJSON(cred{
		Type:       kind,
		Cookie:     cookie,
		Account:    account,
		Password:   password,
		UID:        uid,
		VEI:        vei,
		UploadTier: lanzouDefaultUploadTier,
		BaseURL:    LANZOU_DEFAULT.BaseURL,
		ShareURL:   LANZOU_DEFAULT.ShareURL,
	})
	if displayName == "" {
		displayName = uid
	}
	return &model.TokenInfo{
		TokenFrom:         model.ProviderLanzou,
		AccessToken:       cookie,
		RefreshToken:      refresh,
		TokenType:         "Cookie",
		UserID:            model.BuildUserID(model.ProviderLanzou, uid),
		UserName:          displayName,
		NickName:          displayName,
		Name:              displayName,
		DefaultDriveID:    model.BuildDriveID(model.ProviderLanzou, uid),
		ProviderAccountID: uid,
		ProviderRootID:    "-1",
		DeviceID:          vei,
	}
}

func setLanzouUploadTier(token *model.TokenInfo, tier string) {
	if token == nil {
		return
	}
	cr := parseLanzouCred(token.RefreshToken)
	if cr == nil {
		return
	}
	cr.UploadTier = normalizeLanzouUploadTier(tier)
	token.RefreshToken = mustJSON(cr)
}
