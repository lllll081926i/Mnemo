package app

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func TestThunderQuotaTrashAndShareApplicationFlow(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const user = "thunder_features"
	const space = "features"
	token := &model.TokenInfo{UserID: user, TokenFrom: model.ProviderThunder, AccessToken: "features-access", RefreshToken: `{"refresh_token":"features-refresh","device_id":"0123456789abcdef0123456789abcdef","user_id":"features"}`}
	if err := st.SaveAccount(&model.Account{UserID: user, DriveID: space, Token: token}); err != nil {
		t.Fatal(err)
	}
	drive.SetTokenResolver(func(userID, driveID string) (*model.TokenInfo, error) {
		if userID != user || driveID != space {
			t.Fatalf("迅雷能力使用了错误账号: %s %s", userID, driveID)
		}
		account, err := st.GetAccount(userID)
		if err != nil {
			return nil, err
		}
		return account.Token, nil
	})
	drive.SetTokenUpdater(func(userID, driveID string, fresh *model.TokenInfo) error {
		account, err := st.GetAccount(userID)
		if err != nil {
			return err
		}
		account.Token = fresh
		return st.SaveAccount(account)
	})
	previous := netx.TestTransportHook
	t.Cleanup(func() { drive.SetTokenResolver(nil); drive.SetTokenUpdater(nil); netx.TestTransportHook = previous })
	denyCancel := true
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api-pan.xunlei.com" || request.Header.Get("Authorization") != "Bearer features-access" {
			t.Fatal("迅雷业务请求使用了错误服务或凭据")
		}
		status, body := 200, `{}`
		switch request.URL.Path {
		case "/drive/v1/about":
			body = `{"quota":{"limit":"100","usage":"25"}}`
		case "/drive/v1/files":
			body = `{"files":[{"id":"file","name":"文件","size":25,"trashed":true}]}`
		case "/drive/v1/files/file/untrash":
		case "/drive/v1/share":
			body = `{"share_id":"share","share_url":"https://pan.xunlei.com/s/share?pwd=1234","pass_code":"1234"}`
		case "/drive/v1/share/delete":
			if denyCancel {
				status, body = 403, `{"error":"permission_denied"}`
			}
		default:
			t.Fatalf("意外的迅雷请求: %s %s", request.Method, request.URL)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	application := &App{store: st}
	account, err := application.RefreshAccount(user)
	if err != nil || account.Usage == nil || account.Usage.Size != 100 || account.Usage.Used != 25 || account.Usage.Status != "available" || account.Token.FreeSize != 75 {
		t.Fatalf("迅雷容量未接入账号界面: %+v, %v", account, err)
	}
	files, err := application.ListTrash(user, space)
	if err != nil || len(files) != 1 || files[0].FileID != "file" {
		t.Fatalf("迅雷回收站入口未接通: %+v, %v", files, err)
	}
	if done, err := application.RestoreFiles(user, space, []string{"file"}); err != nil || len(done) != 1 {
		t.Fatalf("迅雷还原入口未接通: %v, %v", done, err)
	}
	item, err := application.CreateShare(user, space, drive.ShareParams{FileIDs: []string{"file"}, ShareName: "测试"})
	if err != nil || item.ShareID != "share" {
		t.Fatalf("迅雷分享入口未接通: %+v, %v", item, err)
	}
	history := application.ListShareHistory(user)
	if len(history) != 1 || history[0].Provider != model.ProviderThunder || history[0].SharePwd != "1234" {
		t.Fatalf("迅雷分享未进入本地记录: %+v", history)
	}
	if err := application.CancelShare(history[0]); err == nil || len(application.ListShareHistory(user)) != 1 {
		t.Fatal("撤销失败不应清除本地分享记录")
	}
	denyCancel = false
	if err := application.CancelShare(history[0]); err != nil || len(application.ListShareHistory(user)) != 0 {
		t.Fatalf("远端撤销成功后未清除本地记录: %v", err)
	}
}
