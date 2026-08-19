package app

import (
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
