package pikpak

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestParseAPIErrorClassifiesRiskControl(t *testing.T) {
	err := parseAPIError([]byte(`{"error":"AccessProhibited"}`), http.StatusForbidden)
	var prohibited *PikPakAccessProhibitedError
	if !errors.As(err, &prohibited) {
		t.Fatalf("error = %v, want access-prohibited classification", err)
	}
	if err := parseAPIError([]byte(`{"message":"Your operation is too frequent, please try again later"}`), http.StatusBadRequest); err == nil {
		t.Fatal("too-frequent response returned nil error")
	} else {
		var rate *PikPakRateLimitError
		if !errors.As(err, &rate) || rate.RetryAfterSeconds < pikpakMinRateLimitSeconds {
			t.Fatalf("error = %v, want rate-limit classification", err)
		}
	}
}

func TestRateLimitErrorExposesAtLeastMinimumCooldown(t *testing.T) {
	var _ interface{ RetryAfter() time.Duration } = (*PikPakRateLimitError)(nil)
	if got := (&PikPakRateLimitError{RetryAfterSeconds: 1}).RetryAfter(); got < time.Duration(pikpakMinRateLimitSeconds)*time.Second {
		t.Fatalf("RetryAfter() = %v, want at least %ds", got, pikpakMinRateLimitSeconds)
	}
}

func TestPikPakCooldownIsScopedToLoginAccount(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	rememberPikPakLoginCooldown("first@example.com", &PikPakRateLimitError{RetryAfterSeconds: 60})
	if err := pikpakLoginCooldownError("first@example.com"); err == nil {
		t.Fatal("the rate-limited account should remain in cooldown")
	}
	if err := pikpakLoginCooldownError("second@example.com"); err != nil {
		t.Fatalf("a different PikPak account was blocked by the first account: %v", err)
	}
}

func TestPikPakRiskCooldownIsLongAndScopedToLoginAccount(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	rememberPikPakLoginCooldown("first@example.com", &PikPakAccessProhibitedError{})

	err := pikpakLoginCooldownError("first@example.com")
	var rate *PikPakRateLimitError
	if !errors.As(err, &rate) || rate.RetryAfterSeconds < pikpakRiskControlCooldownSeconds {
		t.Fatalf("risk cooldown = %v, want at least %d seconds", err, pikpakRiskControlCooldownSeconds)
	}
	if err := pikpakLoginCooldownError("second@example.com"); err != nil {
		t.Fatalf("a different PikPak account was blocked by risk control: %v", err)
	}
}

func TestPikPakVerifiedCaptchaConfirmsOnceBeforeSignIn(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)

	oldWait := pikpakVerifiedCaptchaWait
	pikpakVerifiedCaptchaWait = 0
	t.Cleanup(func() { pikpakVerifiedCaptchaWait = oldWait })
	oldStore := deviceStore
	deviceStore = &storePath{dir: t.TempDir()}
	t.Cleanup(func() { deviceStore = oldStore })
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })

	confirmCalls := 0
	signInCalls := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "user.mypikpak.com/v1/shield/captcha/init":
			confirmCalls++
			var body struct {
				CaptchaToken string `json:"captcha_token"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("decode captcha confirmation: %v", err)
			}
			if body.CaptchaToken != "initial-token" {
				t.Errorf("confirmation token = %q, want initial-token", body.CaptchaToken)
			}
			return pikpakResponse(req, http.StatusOK, `{"captcha_token":"confirmed-token"}`), nil
		case "user.mypikpak.com/v1/auth/signin":
			signInCalls++
			if got := req.Header.Get("X-Captcha-Token"); got != "confirmed-token" {
				t.Errorf("signin captcha token = %q, want confirmed-token", got)
			}
			return pikpakResponse(req, http.StatusOK, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"token_type":"Bearer","sub":"account-1","user_name":"Tester"}`), nil
		case "api-drive.mypikpak.com/drive/v1/about":
			return pikpakResponse(req, http.StatusOK, `{"quota":{"limit":100,"used":25}}`), nil
		default:
			return pikpakResponse(req, http.StatusNotFound, `{}`), nil
		}
	})

	token, err := authSignIn(context.Background(), drive.AuthRequest{Config: map[string]string{
		"username":                      "first@example.com",
		"password":                      "secret",
		"captcha_token":                 "initial-token",
		"captcha_verified":              "true",
		"captcha_requires_confirmation": "true",
		"captcha_redirect_uri":          "http://127.0.0.1:9000/callback",
	}})
	if err != nil {
		t.Fatalf("authSignIn returned error: %v", err)
	}
	if confirmCalls != 1 || signInCalls != 1 {
		t.Fatalf("request counts = confirmation:%d signin:%d, want one each", confirmCalls, signInCalls)
	}
	if token == nil || token.ProviderAccountID != "account-1" || token.UsedSize != 25 || token.TotalSize != 100 {
		t.Fatalf("token = %+v", token)
	}
}

func TestPikPakDeviceIDIsStableAndIndependentPerAccount(t *testing.T) {
	dir := t.TempDir()
	oldStore := deviceStore
	deviceStore = &storePath{dir: dir}
	t.Cleanup(func() { deviceStore = oldStore })

	first := getOrCreateDeviceID("First@example.com")
	firstAgain := getOrCreateDeviceID("first@example.com")
	second := getOrCreateDeviceID("second@example.com")
	if len(first) != 32 || !isHex(first) || first != firstAgain {
		t.Fatalf("device id is not stable per account: first=%q again=%q", first, firstAgain)
	}
	if first == second {
		t.Fatalf("different accounts received the same device id: %q", first)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("device identity directory was not persisted: %v", err)
	}
}

func TestAPIParentIDNormalizesRootSentinels(t *testing.T) {
	for _, value := range []string{"", "root", RootID, "/", "*"} {
		if got := apiParentID(value); got != "" {
			t.Fatalf("apiParentID(%q) = %q, want empty root parent", value, got)
		}
	}
	if got := apiParentID("folder-123"); got != "folder-123" {
		t.Fatalf("apiParentID(folder-123) = %q, want folder-123", got)
	}
}

func TestRootIDNormalizesProviderRoot(t *testing.T) {
	for _, value := range []string{"", "root", RootID, "/"} {
		if got := rootID(value); got != RootID {
			t.Fatalf("rootID(%q) = %q, want %q", value, got, RootID)
		}
	}
}

func TestStreamTypeUsesExplicitHintsAndExtensions(t *testing.T) {
	cases := map[string]struct {
		url  string
		hint string
		want string
	}{
		"generic stream stays mp4": {url: "https://cdn.example/video.mp4", hint: "stream", want: "mp4"},
		"HLS MIME":                 {url: "https://cdn.example/token", hint: "application/vnd.apple.mpegurl", want: "hls"},
		"DASH extension":           {url: "https://cdn.example/video.mpd?signature=secret", want: "dash"},
		"Matroska extension":       {url: "https://cdn.example/video.mkv", want: "mkv"},
		"RealMedia MIME":           {url: "https://cdn.example/token", hint: "video/vnd.rn-realvideo", want: "rmvb"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := streamType(tc.url, tc.hint); got != tc.want {
				t.Fatalf("streamType(%q, %q) = %q, want %q", tc.url, tc.hint, got, tc.want)
			}
		})
	}
}

type pikpakRoundTripper func(*http.Request) (*http.Response, error)

func (f pikpakRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func pikpakResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestCreateShareUsesPikPakDriveAPI(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "api-drive.mypikpak.com" || req.URL.Path != "/drive/v1/share" {
			return nil, errors.New("unexpected PikPak share request")
		}
		if req.Header.Get("Authorization") != "Bearer access-token" || req.Header.Get("X-Device-Id") != "device-test" {
			return nil, errors.New("PikPak share request missing authentication headers")
		}
		var body struct {
			FileIDs        []string `json:"file_ids"`
			ShareTo        string   `json:"share_to"`
			ExpirationDays int      `json:"expiration_days"`
			PassCodeOption string   `json:"pass_code_option"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		if strings.Join(body.FileIDs, ",") != "file-1,folder-1" || body.ShareTo != "encryptedlink" || body.ExpirationDays != 7 || body.PassCodeOption != "REQUIRED" {
			return nil, errors.New("unexpected PikPak share body")
		}
		return pikpakResponse(req, http.StatusOK, `{"share_id":"share-pikpak","share_url":"https://mypikpak.com/s/share-pikpak","pass_code":"p4ss","expiration":"2030-01-01T00:00:00Z","file_ids":["file-1","folder-1"]}`), nil
	})

	item, err := (&Driver{}).CreateShare(context.Background(), drive.Context{
		UserID: "pikpak:account-test", DriveID: "pikpak:account-test",
		Token: &model.TokenInfo{AccessToken: "access-token", DeviceID: "device-test", ProviderAccountID: "account-test"},
	}, drive.ShareParams{FileIDs: []string{"file-1", "folder-1"}, ShareName: "测试分享", Expiration: "7", Password: "p4ss"})
	if err != nil {
		t.Fatalf("CreateShare() error = %v", err)
	}
	if item.ShareID != "share-pikpak" || item.ShareURL != "https://mypikpak.com/s/share-pikpak" || item.SharePwd != "p4ss" || len(item.FileIDList) != 2 {
		t.Fatalf("share = %+v", item)
	}
}
