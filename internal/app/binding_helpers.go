package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

func configKeys(config map[string]string) []string {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func redactID(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:6])
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "invalid"
	}
	return u.Hostname()
}
