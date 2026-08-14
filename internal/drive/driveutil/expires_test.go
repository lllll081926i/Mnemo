package driveutil

import (
	"testing"
	"time"
)

func TestGetExpiresTime(t *testing.T) {
	// pure numeric seconds timestamp (>= 1e9) → *1000
	if got := GetExpiresTime("https://cdn.example/video.mp4?expire=1999999999"); got != 1999999999000 {
		t.Errorf("expire=1999999999 → %d, want 1999999999000", got)
	}
	if got := GetExpiresTime("https://cdn.example/f.mp4?e=1999999999&token=x"); got != 1999999999000 {
		t.Errorf("e=1999999999 → %d, want 1999999999000", got)
	}
	// too-small business param must not be misread as a date
	if got := GetExpiresTime("https://cdn.example/f.mp4?e=1&token=x"); got != 0 {
		t.Errorf("e=1 → %d, want 0", got)
	}
	// AWS X-Amz-Date + X-Amz-Expires
	url := "https://cdn.example/video.mp4?X-Amz-Date=20250101T000000Z&X-Amz-Expires=600"
	want := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli() + 600_000
	if got := GetExpiresTime(url); got != want {
		t.Errorf("x-amz → %d, want %d", got, want)
	}
	// lowercase parameter names also match
	if got := GetExpiresTime("https://cdn.example/f.mp4?expires=1700000000"); got != 1700000000000 {
		t.Errorf("expires=1700000000 → %d, want 1700000000000", got)
	}
	// invalid UTF-8 percent-encoding must not panic and yields 0
	if got := GetExpiresTime("https://cdn.example/video.mp4?token=%E0%A4%A"); got != 0 {
		t.Errorf("invalid utf8 → %d, want 0", got)
	}
	// empty / no expiry params → 0
	if got := GetExpiresTime(""); got != 0 {
		t.Errorf("empty → %d, want 0", got)
	}
	if got := GetExpiresTime("https://cdn.example/f.mp4?token=x"); got != 0 {
		t.Errorf("no expiry → %d, want 0", got)
	}
}
