package captcha

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNativeThunderPageInitialization(t *testing.T) {
	var script string
	view := nativeICoreWebView2{Vtbl: &nativeICoreWebView2Vtbl{}}
	view.Vtbl.ExecuteScript = nativeComProc(windows.NewCallback(func(_ uintptr, source *uint16, _ uintptr) uintptr {
		script = windows.UTF16PtrToString(source)
		return 0
	}))
	window := captchaWindow{ctx: context.Background(), view: &view, session: Session{Provider: "thunder"}}
	window.navigationCompleted(0, nil)
	if !strings.Contains(script, "sdk.init(") || !strings.Contains(script, "xlaccsdk01://xunlei.com/callback?state=harbor") {
		t.Fatal("官方页面加载后没有调用 SDK 并绑定迅雷回调")
	}
	script = ""
	window.session.Provider = "pikpak"
	window.navigationCompleted(0, nil)
	if script != "" {
		t.Fatal("迅雷初始化脚本影响了 PikPak")
	}
}

func TestCaptchaWindowCentersAndStaysInWorkArea(t *testing.T) {
	for _, sample := range []struct {
		name   string
		anchor nativeRECT
		work   nativeRECT
		want   nativeRECT
	}{
		{name: "主窗口居中", anchor: nativeRECT{Left: 180, Top: 80, Right: 1380, Bottom: 880}, work: nativeRECT{Right: 1920, Bottom: 1040}, want: nativeRECT{Left: 520, Top: 120, Right: 1040, Bottom: 840}},
		{name: "负坐标副屏", anchor: nativeRECT{Left: -1800, Top: 70, Right: -600, Bottom: 870}, work: nativeRECT{Left: -1920, Right: 0, Bottom: 1040}, want: nativeRECT{Left: -1460, Top: 110, Right: -940, Bottom: 830}},
		{name: "小屏保留完整窗口", anchor: nativeRECT{Left: 900, Top: 900, Right: 1300, Bottom: 1500}, work: nativeRECT{Left: 20, Top: 30, Right: 420, Bottom: 630}, want: nativeRECT{Left: 20, Top: 30, Right: 420, Bottom: 630}},
		{name: "最大化边框越界", anchor: nativeRECT{Left: -8, Top: -8, Right: 1928, Bottom: 1048}, work: nativeRECT{Right: 1920, Bottom: 1040}, want: nativeRECT{Left: 700, Top: 160, Right: 1220, Bottom: 880}},
		{name: "无主窗口时屏幕居中", anchor: nativeRECT{Right: 1920, Bottom: 1040}, work: nativeRECT{Right: 1920, Bottom: 1040}, want: nativeRECT{Left: 700, Top: 160, Right: 1220, Bottom: 880}},
		{name: "窗口靠右下时限制工作区", anchor: nativeRECT{Left: 1600, Top: 800, Right: 2000, Bottom: 1000}, work: nativeRECT{Right: 1920, Bottom: 1040}, want: nativeRECT{Left: 1400, Top: 320, Right: 1920, Bottom: 1040}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			if got := fitCaptchaWindow(sample.anchor, sample.work); got != sample.want {
				t.Fatalf("验证窗口位置 = %+v，期望 %+v", got, sample.want)
			}
		})
	}
}

func TestNativeThunderCreditKeyIntercept(t *testing.T) {
	results := make(chan VerificationResult, 1)
	session, err := StartForProviderWithResult("thunder", func(_ Session, result VerificationResult) { results <- result })
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	window := captchaWindow{ctx: context.Background(), session: *session}
	if !window.intercept("xlaccsdk01://xunlei.com/callback?state=harbor&creditkey=final-key") {
		t.Fatal("未截获 SDK 验证结果")
	}
	select {
	case result := <-results:
		if result.CreditKey != "final-key" || result.CaptchaToken != "" {
			t.Fatal("SDK 信任参数回传失败")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SDK 验证结果未送达")
	}
}

func TestNativeReportResponseBody(t *testing.T) {
	for _, status := range []int32{200, 401} {
		t.Run(strconv.Itoa(int(status)), func(t *testing.T) {
			results := make(chan string, 1)
			s, err := Start(func(_ Session, token string) { results <- token })
			if err != nil {
				t.Fatal(err)
			}
			defer Close()
			token := strings.Repeat("report-token", 4)
			body := bytes.NewReader([]byte(`{"data":{"captcha_token":"` + token + `"}}`))
			unknown := nativeIUnknownVtbl{Release: nativeComProc(windows.NewCallback(func(uintptr) uintptr { return 1 }))}
			stream := nativeIStream{Vtbl: &nativeStreamVtbl{nativeIUnknownVtbl: unknown}}
			stream.Vtbl.Read = nativeComProc(windows.NewCallback(func(_ uintptr, dst *byte, length uint32, read *uint32) uintptr {
				n, _ := body.Read(unsafe.Slice(dst, int(length)))
				*read = uint32(n)
				if n < int(length) {
					return 1
				}
				return 0
			}))
			request := nativeRequest{Vtbl: &nativeRequestVtbl{nativeIUnknownVtbl: unknown}}
			request.Vtbl.GetUri = nativeComProc(windows.NewCallback(func(_ uintptr, out **uint16) uintptr {
				u := windows.StringToUTF16Ptr("https://user.mypikpak.com/credit/v1/report")
				hr, _, _ := windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHStrDupW").Call(uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(out)))
				return hr
			}))
			response := nativeResponse{Vtbl: &nativeResponseVtbl{nativeIUnknownVtbl: unknown}}
			response.Vtbl.GetStatusCode = nativeComProc(windows.NewCallback(func(_ uintptr, out *int32) uintptr { *out = status; return 0 }))
			type callbackVtbl struct {
				nativeIUnknownVtbl
				Invoke nativeComProc
			}
			type callbackObject struct{ Vtbl *callbackVtbl }
			response.Vtbl.GetContent = nativeComProc(windows.NewCallback(func(_ uintptr, cb *callbackObject) uintptr {
				cb.Vtbl.Invoke.Call(uintptr(unsafe.Pointer(cb)), 0, uintptr(unsafe.Pointer(&stream)))
				return 0
			}))
			args := nativeICoreWebView2WebResourceResponseReceivedEventArgs{Vtbl: &nativeResponseArgsVtbl{nativeIUnknownVtbl: unknown}}
			args.Vtbl.GetRequest = nativeComProc(windows.NewCallback(func(_ uintptr, out **nativeRequest) uintptr { *out = &request; return 0 }))
			args.Vtbl.GetResponse = nativeComProc(windows.NewCallback(func(_ uintptr, out **nativeResponse) uintptr { *out = &response; return 0 }))
			w := captchaWindow{ctx: context.Background(), session: *s}
			w.responseReceived(0, unsafe.Pointer(&args))
			select {
			case got := <-results:
				if status != 200 || got != token {
					t.Fatalf("unexpected result %q for status %d", got, status)
				}
			case <-time.After(100 * time.Millisecond):
				if status == 200 {
					t.Fatal("response body token was not delivered")
				}
			}
		})
	}
}

func TestNativeWebViewCallback(t *testing.T) {
	if os.Getenv("MNEMO_TEST_WEBVIEW2") != "1" {
		t.Skip("set MNEMO_TEST_WEBVIEW2=1 to exercise the installed WebView2 Runtime")
	}
	for _, provider := range []string{"pikpak", "thunder"} {
		t.Run(provider, func(t *testing.T) {
			results := make(chan VerificationResult, 1)
			s, err := StartForProviderWithResult(provider, func(_ Session, result VerificationResult) { results <- result })
			if err != nil {
				t.Fatal(err)
			}
			defer Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			token := strings.Repeat("native-token", 4)
			callback := s.CallbackURL + "#captcha_token=" + token
			expected := VerificationResult{CaptchaToken: token}
			if provider == "thunder" {
				callback = s.CallbackURL + "#creditkey=verified-credit-key"
				expected = VerificationResult{CreditKey: "verified-credit-key"}
			}
			profile, err := os.MkdirTemp("", "mnemo-captcha-test-")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cancel()
				for i := 0; i < 50; i++ {
					if os.RemoveAll(profile) == nil {
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				t.Errorf("WebView2 profile still in use: %s", profile)
			}()
			// The hidden real WebView exercises COM setup, response-event registration,
			// navigation interception and fragment delivery without an online account.
			if err := startNativeWindow(ctx, *s, callback, profile, false); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-results:
				if got != expected {
					t.Fatal("原生验证结果未正确回传")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("native callback did not arrive")
			}
		})
	}
}
