//go:build !windows

package netx

func systemProxyURL() string { return "" }
