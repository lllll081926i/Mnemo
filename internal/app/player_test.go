package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"mnemo-go/internal/model"
)

func TestExpirationTimeUsesEarliestKnownValue(t *testing.T) {
	got := expirationTime(1_800_000_300, 1_800_000_100_000)
	want := time.UnixMilli(1_800_000_100_000)
	if !got.Equal(want) {
		t.Fatalf("expirationTime() = %v, want %v", got, want)
	}
}

func TestSourceExpirationPrefersEarlierQualityExpiry(t *testing.T) {
	preview := &model.VideoPreview{ExpireTime: 1_800_000_500}
	quality := model.VideoQuality{ExpireTime: 1_800_000_100}
	got := sourceExpiration(preview, quality)
	want := time.UnixMilli(1_800_000_100_000)
	if !got.Equal(want) {
		t.Fatalf("sourceExpiration() = %v, want %v", got, want)
	}
}

func TestSanitizeVideoPreviewHidesProviderCredentials(t *testing.T) {
	preview := &model.VideoPreview{
		Headers: map[string]string{"Authorization": "Bearer secret"},
		Qualities: []model.VideoQuality{{
			HTML:    "https://provider.example/play?signature=secret",
			URL:     "https://provider.example/play?signature=secret",
			Headers: map[string]string{"Cookie": "secret"},
			Label:   "原画",
			Value:   "origin",
		}},
	}
	sanitizeVideoPreview(preview)
	if preview.Headers != nil {
		t.Fatalf("preview headers were not cleared: %#v", preview.Headers)
	}
	quality := preview.Qualities[0]
	if quality.HTML != "" || quality.URL != "" || quality.Headers != nil {
		t.Fatalf("provider details were not cleared: %#v", quality)
	}
	if quality.Label != "原画" || quality.Value != "origin" {
		t.Fatalf("player selection metadata changed: %#v", quality)
	}
}

func TestVideoStreamType(t *testing.T) {
	cases := map[string]struct {
		declared string
		filename string
		want     string
	}{
		"declared hls":          {declared: "m3u8", filename: "video.mp4", want: "hls"},
		"dash extension":        {filename: "video.mpd", want: "dash"},
		"unsupported container": {filename: "video.mkv", want: "mkv"},
		"native mp4":            {filename: "video.mov", want: "mp4"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := videoStreamType(tc.declared, tc.filename); got != tc.want {
				t.Fatalf("videoStreamType(%q, %q) = %q, want %q", tc.declared, tc.filename, got, tc.want)
			}
		})
	}
}

func TestSyncAccountUsageBuildsDisplayQuota(t *testing.T) {
	acc := &model.Account{Token: &model.TokenInfo{TotalSize: 4096, FreeSize: 3072}}
	syncAccountUsage(acc)
	if acc.Usage == nil {
		t.Fatal("syncAccountUsage() did not build quota")
	}
	if acc.Usage.Size != 4096 || acc.Usage.Used != 1024 {
		t.Fatalf("quota = %#v, want size=4096 used=1024", acc.Usage)
	}
	if acc.Usage.SizeStr == "" || acc.Usage.UsedStr == "" {
		t.Fatalf("formatted quota is incomplete: %#v", acc.Usage)
	}
}

func TestAccountRefreshRiskFailureUsesCooldown(t *testing.T) {
	a := NewApp()
	a.markAccountRefreshFailure("pikpak_test", errors.New("HTTP 429 rate limited"))
	if !a.accountRefreshCached("pikpak_test") {
		t.Fatal("risk-control refresh failure did not enter cooldown")
	}
}

func TestSyncRunRegistryRejectsOverlapAndCancels(t *testing.T) {
	a := NewApp()
	ctx, finish, err := a.beginSyncRun(context.Background(), "sync-test", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.beginSyncRun(context.Background(), "sync-test", "scheduler"); err == nil {
		t.Fatal("overlapping sync run should be rejected")
	}
	ids := a.ListRunningSyncIDs()
	if len(ids) != 1 || ids[0] != "sync-test" {
		t.Fatalf("running sync IDs = %#v", ids)
	}
	if !a.CancelSync("sync-test") {
		t.Fatal("CancelSync should accept an active job")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("sync context was not canceled")
	}
	if !a.isSyncRunning("sync-test") {
		t.Fatal("registry should retain canceled job until worker exits")
	}
	finish(context.Canceled)
	if a.isSyncRunning("sync-test") || a.CancelSync("sync-test") {
		t.Fatal("finished sync job remained active")
	}
}

func TestValidateExternalBrowserURL(t *testing.T) {
	for _, allowed := range []string{"https://example.com/help", "http://127.0.0.1:1234/callback", "http://localhost:1234/callback"} {
		if _, err := validateExternalBrowserURL(allowed); err != nil {
			t.Errorf("validateExternalBrowserURL(%q): %v", allowed, err)
		}
	}
	for _, rejected := range []string{"http://example.com/help", "file:///C:/Windows/win.ini", "custom:payload", "https://user:password@example.com/", "/relative"} {
		if _, err := validateExternalBrowserURL(rejected); err == nil {
			t.Errorf("validateExternalBrowserURL(%q) should reject", rejected)
		}
	}
}
