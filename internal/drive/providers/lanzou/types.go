// Package lanzou implements the 蓝奏云 (woozooo.com) drive provider, ported
// from the legacy Electron TypeScript implementation (src/lanzou + the
// lanzou drive adapter). It uses the woozooo doupload.php task API, the
// acw_sc__v2 anti-bot challenge solver and the share-page download chain.
package lanzou

import (
	"regexp"
	"strconv"
	"strings"
)

// LANZOU_ROOT is the provider root folder id surfaced to the frontend.
const LANZOU_ROOT = "lanzou_root"

// LANZOU_DEFAULT mirrors the legacy defaults.
var LANZOU_DEFAULT = struct {
	BaseURL   string
	ShareURL  string
	UserAgent string
}{
	BaseURL:   "https://pc.woozooo.com",
	ShareURL:  "https://pan.lanzoui.com",
	UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
}

// LanzouItem is one raw list entry returned by the doupload task APIs.
type LanzouItem struct {
	ID      string `json:"id"`
	FolID   string `json:"fol_id"`
	Name    string `json:"name"`
	NameAll string `json:"name_all"`
	Size    string `json:"size"`
	Time    string `json:"time"`
}

// ToLanzouFolderId maps a user-facing folder id to the API folder id
// ('-1' = root). Empty/root sentinels all collapse to the root.
func ToLanzouFolderId(id string) string {
	if id == "" || id == LANZOU_ROOT || id == "root" || id == "/" {
		return "-1"
	}
	return id
}

var sizeRe = regexp.MustCompile(`(?i)^([\d.]+)\s*([bkmgt]?b?)?$`)

// parseSizeToBytes converts a lanzou size string ("1.5 M", "10K", "2B") to bytes.
func parseSizeToBytes(sizeStr string) int64 {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(sizeStr))
	if m == nil {
		return 0
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(m[2])
	if unit == "" {
		unit = "b"
	}
	switch {
	case strings.HasPrefix(unit, "t"):
		return int64(n * 1024 * 1024 * 1024 * 1024)
	case strings.HasPrefix(unit, "g"):
		return int64(n * 1024 * 1024 * 1024)
	case strings.HasPrefix(unit, "m"):
		return int64(n * 1024 * 1024)
	case strings.HasPrefix(unit, "k"):
		return int64(n * 1024)
	default:
		return int64(n)
	}
}
