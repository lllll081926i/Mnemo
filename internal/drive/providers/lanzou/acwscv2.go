package lanzou

import (
	"regexp"
	"strings"
)

// The ACW sc_v2 challenge solver (源码级移植自 AList lanzou help.go 的
// CalcAcwScV2 / Unbox / HexXor 算法)。蓝奏系（woozooo）在访问频繁时返回盾挑战页，
// 页面 JS 脚本算出 acw_sc__v2 值后写入 Cookie 重放才给正常响应。

const acwXORKey = "3000176000856006061501533003690027800375"

var acwBox = [40]int{6, 28, 34, 31, 33, 18, 30, 23, 9, 8, 19, 38, 17, 24, 0, 5, 32, 21, 10, 22, 25, 14, 15, 3, 16, 27, 13, 35, 2, 29, 11, 26, 4, 36, 1, 39, 37, 7, 20, 12}

var acwArg1Re = regexp.MustCompile(`arg1='([0-9A-Fa-f]+)'`)

// solveAcwScV2 parses the challenge page and returns the acw_sc__v2 cookie
// value. Returns "" when the page is not a challenge page.
func solveAcwScV2(html string) string {
	m := acwArg1Re.FindStringSubmatch(html)
	if len(m) < 2 || m[1] == "" {
		return ""
	}
	arg1 := strings.ToUpper(m[1])
	return hexXor(unbox(arg1), acwXORKey)
}

// isAcwScV2Challenge reports whether the response body is a challenge page.
func isAcwScV2Challenge(html string) bool {
	return acwArg1Re.MatchString(html)
}

// mergeAcwCookie merges (or replaces) the acw_sc__v2 value into the existing
// Cookie header value.
func mergeAcwCookie(cookie, acwValue string) string {
	parts := strings.Split(cookie, ";")
	filtered := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" || strings.HasPrefix(p, "acw_sc__v2=") {
			continue
		}
		filtered = append(filtered, p)
	}
	filtered = append(filtered, "acw_sc__v2="+acwValue)
	return strings.Join(filtered, "; ")
}

func unbox(hex string) string {
	out := make([]byte, len(hex))
	for i := 0; i < len(acwBox) && i < len(hex); i++ {
		j := acwBox[i]
		if j < len(out) {
			out[j] = hex[i]
		}
	}
	return string(out)
}

func hexXor(hex1, hex2 string) string {
	var b strings.Builder
	b.Grow(len(hex1))
	l := len(hex1)
	if len(hex2) < l {
		l = len(hex2)
	}
	for i := 0; i+2 <= l; i += 2 {
		v1, _ := hexToByte(hex1[i : i+2])
		v2, _ := hexToByte(hex2[i : i+2])
		b.Write(byteToHex(v1 ^ v2))
	}
	return b.String()
}

func hexToByte(s string) (byte, error) {
	n, err := parseHex(s)
	return byte(n), err
}

func parseHex(s string) (int, error) {
	if len(s) != 2 {
		return 0, nil
	}
	var v int
	for _, c := range []byte(s) {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'A' && c <= 'F':
			v |= int(c - 'A' + 10)
		case c >= 'a' && c <= 'f':
			v |= int(c - 'a' + 10)
		default:
			return 0, nil
		}
	}
	return v, nil
}

func byteToHex(b byte) []byte {
	const hexDigits = "0123456789abcdef"
	return []byte{hexDigits[b>>4], hexDigits[b&0x0f]}
}
