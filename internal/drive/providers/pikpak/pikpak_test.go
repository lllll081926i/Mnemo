package pikpak

import "testing"

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
