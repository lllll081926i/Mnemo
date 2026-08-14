package model

import (
	"fmt"
	"time"
)

// FormatBytes renders bytes as a human-readable string.
func FormatBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	val := float64(n)
	i := 0
	for val >= 1024 && i < len(units)-1 {
		val /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", val, units[i])
}

// FormatTime renders a unix timestamp as "2006-01-02 15:04".
func FormatTime(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

// FormatSpeed renders bytes/s as a human-readable speed.
func FormatSpeed(bps int64) string {
	if bps <= 0 {
		return "0 B/s"
	}
	return FormatBytes(bps) + "/s"
}
