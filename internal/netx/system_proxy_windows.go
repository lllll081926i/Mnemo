//go:build windows

package netx

import "golang.org/x/sys/windows/registry"

const internetSettingsRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// systemProxyURL reads the active per-user manual proxy. PAC/WPAD settings do
// not expose a static proxy URL and are therefore deliberately not guessed.
func systemProxyURL() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return ""
	}
	raw, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return ""
	}
	return normalizeSystemProxy(raw)
}
