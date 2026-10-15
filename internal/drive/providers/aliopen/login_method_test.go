package aliopen

import (
	"mnemo-go/internal/drive"
	"testing"
)

func TestLoginMethodRejectsUndisclosedRelayAndMissingOwnClient(testContext *testing.T) {
	for _, config := range []map[string]string{
		{"refresh_token": "secret", "login_method": "relay"},
		{"refresh_token": "secret", "login_method": "relay", "relay_consent": "true", "client_id": "own"},
		{"refresh_token": "secret", "login_method": "own"},
		{"refresh_token": "secret", "login_method": "unknown"},
	} {
		if _, err := authRefreshToken(testContext.Context(), drive.AuthRequest{Config: config}); err == nil {
			testContext.Fatal("未校验授权来源")
		}
	}
	registration := drive.Require(providerID)
	if len(registration.Login.Methods) != 2 || registration.Login.Methods[0].Recipient != oauthDefault || registration.Login.Methods[1].Recipient != apiHost+"/oauth/access_token" {
		testContext.Fatal("声明的令牌接收方与实际实现不一致")
	}
}
