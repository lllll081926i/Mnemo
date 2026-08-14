package ilanzou

import (
	"context"
	"errors"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// authLogin is the registration AuthFunc (port of legacy ilanzou auth.ts).
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	user := strings.TrimSpace(req.Config["username"])
	pass := req.Config["password"]
	if user == "" || pass == "" {
		return nil, errors.New("请输入优享版蓝奏云账号和密码")
	}
	login, err := ilanzouLogin(ctx, user, pass, "")
	if err != nil {
		return nil, err
	}
	id := login.userId
	if id == "" {
		id = user
	}
	name := login.account
	if name == "" {
		name = user
	}
	refresh := mustJSON(cred{
		Username: user,
		Password: pass,
		UUID:     login.uuid,
		Token:    login.token,
		UserID:   login.userId,
		Account:  login.account,
	})
	return &model.TokenInfo{
		TokenFrom:         model.ProviderIlanzou,
		AccessToken:       login.token,
		RefreshToken:      refresh,
		TokenType:         "Bearer",
		UserID:            model.BuildUserID(model.ProviderIlanzou, id),
		UserName:          name,
		NickName:          name,
		Name:              name,
		DefaultDriveID:    model.BuildDriveID(model.ProviderIlanzou, id),
		ProviderAccountID: id,
		ProviderRootID:    "0",
		DeviceID:          login.uuid,
	}, nil
}
