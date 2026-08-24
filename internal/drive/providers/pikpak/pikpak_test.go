package pikpak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		if !errors.As(err, &rate) || rate.RetryAfterSeconds != pikpakDefaultRateLimitSeconds {
			t.Fatalf("error = %v, want rate-limit classification", err)
		}
	}
}

func TestRateLimitErrorPreservesShortServerCooldown(t *testing.T) {
	var _ interface{ RetryAfter() time.Duration } = (*PikPakRateLimitError)(nil)
	err := parseAPIErrorWithRetry([]byte(`{"error":"too_many_requests","retry_after":5}`), http.StatusTooManyRequests, "")
	var rate *PikPakRateLimitError
	if !errors.As(err, &rate) || rate.RetryAfterSeconds != 5 {
		t.Fatalf("error = %v, want server retry-after of 5 seconds", err)
	}
	if got := rate.RetryAfter(); got != 5*time.Second {
		t.Fatalf("RetryAfter() = %v, want 5s", got)
	}
}

func TestPikPakCooldownIsScopedToLoginAccount(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	rememberPikPakLoginCooldown("first@example.com", &PikPakRateLimitError{RetryAfterSeconds: 60})
	err := pikpakLoginCooldownError("first@example.com")
	if err == nil {
		t.Fatal("the rate-limited account should remain in cooldown")
	}
	var rate *PikPakRateLimitError
	if !errors.As(err, &rate) || rate.RetryAfterSeconds <= 0 || rate.RetryAfterSeconds > pikpakLoginCooldownCapSeconds {
		t.Fatalf("cooldown = %v, want no more than %d seconds", err, pikpakLoginCooldownCapSeconds)
	}
	if err := pikpakLoginCooldownError("second@example.com"); err != nil {
		t.Fatalf("a different PikPak account was blocked by the first account: %v", err)
	}
}

func TestPikPakRiskCooldownIsCappedAndScopedToLoginAccount(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	rememberPikPakLoginCooldown("first@example.com", &PikPakAccessProhibitedError{})

	err := pikpakLoginCooldownError("first@example.com")
	var rate *PikPakRateLimitError
	if !errors.As(err, &rate) || rate.RetryAfterSeconds <= 0 || rate.RetryAfterSeconds > pikpakLoginCooldownCapSeconds {
		t.Fatalf("risk cooldown = %v, want no more than %d seconds", err, pikpakLoginCooldownCapSeconds)
	}
	if err := pikpakLoginCooldownError("second@example.com"); err != nil {
		t.Fatalf("a different PikPak account was blocked by risk control: %v", err)
	}
}

func TestPikPakLongCooldownIsNotStoredLocally(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	rememberPikPakLoginCooldown("first@example.com", &PikPakRateLimitError{RetryAfterSeconds: pikpakLoginCooldownIgnoreAtSeconds})
	if err := pikpakLoginCooldownError("first@example.com"); err != nil {
		t.Fatalf("long server retry must not lock the local login button: %v", err)
	}
}

func TestPikPakVerifiedCaptchaSignsInWithoutExtraExchange(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)

	oldStore := deviceStore
	deviceStore = &storePath{dir: t.TempDir()}
	t.Cleanup(func() { deviceStore = oldStore })
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })

	captchaCalls := 0
	signInCalls := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "user.mypikpak.com/v1/shield/captcha/init":
			captchaCalls++
			return pikpakResponse(req, http.StatusInternalServerError, `{}`), nil
		case "user.mypikpak.com/v1/auth/signin":
			signInCalls++
			if got := req.Header.Get("X-Captcha-Token"); got != "verified-token" {
				t.Errorf("signin captcha token = %q, want verified-token", got)
			}
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("decode signin: %v", err)
			}
			if _, ok := body["captcha_token"]; ok {
				t.Errorf("signin body must not contain captcha_token: %s", body["captcha_token"])
			}
			if _, ok := body["device_id"]; ok {
				t.Errorf("signin body must not contain device_id: %s", body["device_id"])
			}
			if len(body) != 3 {
				t.Errorf("signin body field count = %d, want only client_id/username/password", len(body))
			}
			for _, field := range []string{"client_id", "username", "password"} {
				if _, ok := body[field]; !ok {
					t.Errorf("signin body is missing %q", field)
				}
			}
			return pikpakResponse(req, http.StatusOK, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"token_type":"Bearer","sub":"account-1","user_name":"Tester"}`), nil
		case "api-drive.mypikpak.com/drive/v1/about":
			return pikpakResponse(req, http.StatusOK, `{"quota":{"limit":100,"used":25}}`), nil
		default:
			return pikpakResponse(req, http.StatusNotFound, `{}`), nil
		}
	})

	token, err := authSignIn(context.Background(), drive.AuthRequest{Config: map[string]string{
		"username":         "first@example.com",
		"password":         "secret",
		"captcha_token":    "verified-token",
		"captcha_verified": "true",
	}})
	if err != nil {
		t.Fatalf("authSignIn returned error: %v", err)
	}
	if captchaCalls != 0 || signInCalls != 1 {
		t.Fatalf("request counts = captcha:%d signin:%d, want no captcha exchange and one signin", captchaCalls, signInCalls)
	}
	if token == nil || token.ProviderAccountID != "account-1" || token.UsedSize != 25 || token.TotalSize != 100 {
		t.Fatalf("token = %+v", token)
	}
}

// TestPikPakLiveLogin is opt-in because it talks to the real provider. It uses
// a temporary identity directory so diagnosis never reuses or overwrites a
// device id from the desktop application.
func TestPikPakLiveLogin(t *testing.T) {
	username := strings.TrimSpace(os.Getenv("MNEMO_PIKPAK_TEST_USERNAME"))
	password := os.Getenv("MNEMO_PIKPAK_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Skip("set MNEMO_PIKPAK_TEST_USERNAME and MNEMO_PIKPAK_TEST_PASSWORD to run the live login test")
	}

	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	oldStore := deviceStore
	deviceStore = &storePath{dir: t.TempDir()}
	t.Cleanup(func() { deviceStore = oldStore })

	token, err := authSignIn(context.Background(), drive.AuthRequest{Config: map[string]string{
		"username": username,
		"password": password,
	}})
	if err != nil {
		var challenge *CaptchaRequiredError
		if errors.As(err, &challenge) {
			t.Fatalf("live login requires an interactive captcha challenge (token and URL intentionally omitted)")
		}
		t.Fatalf("live login failed: %v", err)
	}
	if token == nil || token.AccessToken == "" || token.ProviderAccountID == "" {
		t.Fatalf("live login returned an incomplete token")
	}
}

func TestPikPakCaptchaInvalidRetriesOnceLikeRclone(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)

	oldStore := deviceStore
	deviceStore = &storePath{dir: t.TempDir()}
	t.Cleanup(func() { deviceStore = oldStore })
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })

	captchaCalls := 0
	signInCalls := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "user.mypikpak.com/v1/shield/captcha/init":
			captchaCalls++
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("decode captcha request: %v", err)
			}
			if _, ok := body["captcha_token"]; ok {
				t.Errorf("rclone retry must invalidate the old captcha token: %s", body["captcha_token"])
			}
			return pikpakResponse(req, http.StatusOK, fmt.Sprintf(`{"captcha_token":"captcha-%d"}`, captchaCalls)), nil
		case "user.mypikpak.com/v1/auth/signin":
			signInCalls++
			if signInCalls == 1 {
				return pikpakResponse(req, http.StatusBadRequest, `{"error":"captcha_invalid","error_code":4002}`), nil
			}
			if got := req.Header.Get("X-Captcha-Token"); got != "captcha-2" {
				t.Errorf("retry signin captcha token = %q, want captcha-2", got)
			}
			return pikpakResponse(req, http.StatusOK, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"token_type":"Bearer","sub":"account-1"}`), nil
		case "api-drive.mypikpak.com/drive/v1/about":
			return pikpakResponse(req, http.StatusOK, `{"quota":{"limit":100,"used":25}}`), nil
		default:
			return pikpakResponse(req, http.StatusNotFound, `{}`), nil
		}
	})

	token, err := authSignIn(context.Background(), drive.AuthRequest{Config: map[string]string{
		"username": "first@example.com",
		"password": "secret",
	}})
	if err != nil {
		t.Fatalf("authSignIn returned error: %v", err)
	}
	if captchaCalls != 2 || signInCalls != 2 {
		t.Fatalf("request counts = captcha:%d signin:%d, want two each", captchaCalls, signInCalls)
	}
	if token == nil || token.ProviderAccountID != "account-1" {
		t.Fatalf("token = %+v", token)
	}
}

func TestPikPakAccessProhibitedDoesNotRetryWithoutChallenge(t *testing.T) {
	ResetPikPakLoginCooldown()
	t.Cleanup(ResetPikPakLoginCooldown)
	oldStore := deviceStore
	deviceStore = &storePath{dir: t.TempDir()}
	t.Cleanup(func() { deviceStore = oldStore })
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })

	var captchaCalls, signInCalls int
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "user.mypikpak.com/v1/shield/captcha/init":
			captchaCalls++
			return pikpakResponse(req, http.StatusOK, `{"captcha_token":"plain-token"}`), nil
		case "user.mypikpak.com/v1/auth/signin":
			signInCalls++
			return pikpakResponse(req, http.StatusForbidden, `{"error":"AccessProhibited"}`), nil
		default:
			return pikpakResponse(req, http.StatusNotFound, `{}`), nil
		}
	})

	_, err := authSignIn(context.Background(), drive.AuthRequest{Config: map[string]string{
		"username": "risk@example.com",
		"password": "secret",
	}})
	var prohibited *PikPakAccessProhibitedError
	if !errors.As(err, &prohibited) {
		t.Fatalf("error = %v, want provider risk-control error", err)
	}
	if captchaCalls != 1 || signInCalls != 1 {
		t.Fatalf("request counts = captcha:%d signin:%d, want one captcha init and one signin without retry", captchaCalls, signInCalls)
	}
}

func TestPikPakInitialCaptchaMatchesRcloneProtocol(t *testing.T) {
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })

	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "user.mypikpak.com" || req.URL.Path != "/v1/shield/captcha/init" {
			return nil, fmt.Errorf("unexpected captcha request: %s %s", req.Method, req.URL)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("decode initial captcha request: %w", err)
		}
		if _, ok := body["captcha_token"]; ok {
			return nil, fmt.Errorf("initial captcha request must omit captcha_token: %s", body["captcha_token"])
		}
		if _, ok := body["redirect_uri"]; ok {
			return nil, fmt.Errorf("rclone login captcha request must omit redirect_uri: %s", body["redirect_uri"])
		}
		var meta map[string]string
		if err := json.Unmarshal(body["meta"], &meta); err != nil {
			return nil, fmt.Errorf("decode captcha meta: %w", err)
		}
		if len(meta) != 1 || meta["username"] != "first@example.com" {
			return nil, fmt.Errorf("captcha meta = %#v, want rclone meta.username only", meta)
		}
		return pikpakResponse(req, http.StatusOK, `{"captcha_token":"initial-token"}`), nil
	})

	token, challengeURL, err := initCaptcha(
		context.Background(),
		netx.NewClient(5*time.Second),
		"0123456789abcdef0123456789abcdef",
		"first@example.com",
		"POST:/v1/auth/signin",
	)
	if err != nil {
		t.Fatalf("initCaptcha returned error: %v", err)
	}
	if token != "initial-token" || challengeURL != "" {
		t.Fatalf("initCaptcha result = token:%q challenge:%q", token, challengeURL)
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

func TestPikPakDirectDownloadAuthorizationIsHostScoped(t *testing.T) {
	for rawURL, want := range map[string]bool{
		"https://api-drive.mypikpak.com/drive/v1/files/a/download": true,
		"https://cdn.mypikpak.com/object?a=1":                      false,
		"https://oss.example.test/object?a=1":                      false,
		"not a url":                                                false,
	} {
		if got := pikpakDownloadNeedsAuthorization(rawURL); got != want {
			t.Fatalf("pikpakDownloadNeedsAuthorization(%q) = %v, want %v", rawURL, got, want)
		}
	}
}

func TestComputeGCIDMatchesReferenceAndIsFortyHexCharacters(t *testing.T) {
	payload := []byte("mnemo-pikpak-gcid")
	path := t.TempDir() + "/payload.bin"
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := computeGCID(path, nil)
	if err != nil {
		t.Fatalf("computeGCID() error = %v", err)
	}
	want := strings.ToUpper(gcidFromBytes(payload))
	if got != want || len(got) != 40 {
		t.Fatalf("computeGCID() = %q (len=%d), want %q", got, len(got), want)
	}
}

func TestPikPakDeclaresAndMapsGCID(t *testing.T) {
	caps := drive.RegistryCaps(providerID)
	if strings.Join(caps.ProvideHashes, ",") != "gcid" || strings.Join(caps.RapidUploadHashes, ",") != "gcid" {
		t.Fatalf("PikPak hashes = provide:%v rapid:%v", caps.ProvideHashes, caps.RapidUploadHashes)
	}
	f := mapFile(&File{ID: "file-1", Name: "movie.mkv", Kind: "drive#file", Hash: strings.Repeat("A", 40)}, "drive", RootID)
	if f.ContentHashName != "gcid" || f.ContentHash != strings.Repeat("a", 40) {
		t.Fatalf("mapped hash = %q/%q", f.ContentHashName, f.ContentHash)
	}
}

func TestPikPakRapidUploadByGCIDHitAndMissCleanup(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	createCalls := 0
	deleteCalls := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.Method + " " + req.URL.Path {
		case http.MethodGet + " /drive/v1/files":
			return pikpakResponse(req, http.StatusOK, `{"files":[]}`), nil
		case http.MethodPost + " /drive/v1/files":
			createCalls++
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, err
			}
			if body["hash"] != strings.Repeat("A", 40) || body["name"] != "movie.mkv" {
				return nil, fmt.Errorf("unexpected rapid body: %#v", body)
			}
			if createCalls == 1 {
				return pikpakResponse(req, http.StatusOK, `{"file":{"id":"rapid-file","phase":"PHASE_TYPE_COMPLETE"}}`), nil
			}
			return pikpakResponse(req, http.StatusOK, `{"file":{"id":"pending-file"},"resumable":{"params":{"endpoint":"oss.example"}}}`), nil
		case http.MethodPost + " /drive/v1/files:batchDelete":
			deleteCalls++
			return pikpakResponse(req, http.StatusOK, `{}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
	})
	driver := &Driver{}
	ctx := drive.Context{Token: &model.TokenInfo{AccessToken: "access", DeviceID: "device", ProviderAccountID: "account"}}
	req := drive.RapidUploadRequest{ParentID: RootID, FileName: "movie.mkv", Method: "gcid", Hash: strings.Repeat("a", 40), Size: 4096}
	hit, err := driver.RapidUploadByHash(context.Background(), ctx, req)
	if err != nil || hit == nil || !hit.Reuse || hit.FileID != "rapid-file" {
		t.Fatalf("rapid hit = %+v, %v", hit, err)
	}
	miss, err := driver.RapidUploadByHash(context.Background(), ctx, req)
	if err != nil || miss == nil || miss.Reuse || deleteCalls != 1 {
		t.Fatalf("rapid miss = %+v, err=%v, deleteCalls=%d", miss, err, deleteCalls)
	}
}

func TestPikPakRapidUploadRejectsAmbiguousResponse(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.Method + " " + req.URL.Path {
		case http.MethodGet + " /drive/v1/files":
			return pikpakResponse(req, http.StatusOK, `{"files":[]}`), nil
		case http.MethodPost + " /drive/v1/files":
			return pikpakResponse(req, http.StatusOK, `{"upload_type":"UPLOAD_TYPE_RESUMABLE","file":{"id":"pending-file","phase":"PHASE_TYPE_PENDING"}}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
	})

	result, err := (&Driver{}).RapidUploadByHash(context.Background(), drive.Context{
		UserID: "pikpak:user", DriveID: "pikpak:user",
		Token: &model.TokenInfo{AccessToken: "access", DeviceID: "device", ProviderAccountID: "account"},
	}, drive.RapidUploadRequest{
		ParentID: RootID, FileName: "pending.bin", Size: 7, Method: "gcid", Hash: strings.Repeat("A", 40),
	})
	if err == nil || !strings.Contains(err.Error(), "状态不明确") {
		t.Fatalf("RapidUploadByHash() result=%+v error=%v, want ambiguous-state error", result, err)
	}
}

func TestPikPakOrdinaryUploadRejectsAndCleansAmbiguousResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	deleteCalls := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.Method + " " + req.URL.Path {
		case http.MethodGet + " /drive/v1/files":
			return pikpakResponse(req, http.StatusOK, `{"files":[]}`), nil
		case http.MethodPost + " /drive/v1/files":
			return pikpakResponse(req, http.StatusOK, `{"upload_type":"UPLOAD_TYPE_RESUMABLE","file":{"id":"pending-file","phase":"PHASE_TYPE_PENDING"}}`), nil
		case http.MethodPost + " /drive/v1/files:batchDelete":
			deleteCalls++
			return pikpakResponse(req, http.StatusOK, `{}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
	})
	ui := &model.UploadingUI{Info: model.UploadInfo{
		LocalFilePath: path, ParentFileID: RootID, Name: "payload.bin", ConflictPolicy: "rename",
	}}
	err := (&Driver{}).UploadOneFile(context.Background(), drive.Context{
		Token: &model.TokenInfo{AccessToken: "access", DeviceID: "device", ProviderAccountID: "account"},
	}, ui)
	if err == nil || !strings.Contains(err.Error(), "neither a completed file") {
		t.Fatalf("UploadOneFile() error = %v", err)
	}
	if deleteCalls != 1 || !ui.Upload.IsFailed || ui.Upload.IsCompleted {
		t.Fatalf("deleteCalls=%d upload=%+v", deleteCalls, ui.Upload)
	}
}

func TestPikPakResolveTransferHashUsesMetadataGCID(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/drive/v1/files/file-with-gcid" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
		return pikpakResponse(req, http.StatusOK, `{"id":"file-with-gcid","name":"movie.mkv","kind":"drive#file","phase":"PHASE_TYPE_COMPLETE","hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","links":{"application/octet-stream":{"url":"https://cdn.example/movie"}}}`), nil
	})
	hash, err := (&Driver{}).ResolveTransferHash(context.Background(), drive.Context{
		Token: &model.TokenInfo{AccessToken: "metadata-access", DeviceID: "metadata-device", ProviderAccountID: "metadata-account"},
	}, "file-with-gcid", "gcid", true)
	if err != nil || hash != strings.Repeat("a", 40) {
		t.Fatalf("ResolveTransferHash() = %q, %v", hash, err)
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

func TestPikPakCancelShareUsesBatchDelete(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "api-drive.mypikpak.com" || req.URL.Path != "/drive/v1/share:batchDelete" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer access-token" || req.Header.Get("X-Device-Id") != "device-test" {
			return nil, errors.New("PikPak cancellation missing authentication headers")
		}
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		if strings.Join(body.IDs, ",") != "share-pikpak" {
			return nil, fmt.Errorf("cancellation body = %+v", body)
		}
		return pikpakResponse(req, http.StatusOK, `{}`), nil
	})

	err := (&Driver{}).CancelShare(context.Background(), drive.Context{
		Token: &model.TokenInfo{AccessToken: "access-token", DeviceID: "device-test", ProviderAccountID: "account-test"},
	}, model.ShareHistoryEntry{ShareID: "share-pikpak"})
	if err != nil {
		t.Fatalf("CancelShare() error = %v", err)
	}
}
