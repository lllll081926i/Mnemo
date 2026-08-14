package lanzou

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 蓝奏分享/网盘页解析工具（对齐 AList lanzou help.go）。

// ---- 页面清理（AList help.go） ----

var notesRe = regexp.MustCompile(`<!--[\s\S]*?-->|[^:]//[^\n]*|/\*[\s\S]*?\*/`)

// removeNotes strips HTML/JS comments while preserving "https://" style URLs.
func removeNotes(html string) string {
	return notesRe.ReplaceAllStringFunc(html, func(b string) string {
		// "[^:]//..." match: keep the character preceding the comment markers.
		if len(b) >= 3 && b[1] == '/' && b[2] == '/' {
			return b[:1]
		}
		return "\n"
	})
}

// removeJSComment does a character-by-character scan to drop comments,
// preserving string literals that contain comment markers.
func removeJSComment(data string) string {
	var out strings.Builder
	inComment := false
	inSingleLine := false
	for i := 0; i < len(data); i++ {
		v := data[i]
		if inSingleLine {
			if v == '\n' || v == '\r' {
				inSingleLine = false
				out.WriteByte(v)
			}
			continue
		}
		if inComment {
			if v == '*' && i+1 < len(data) && data[i+1] == '/' {
				inComment = false
				i++
			}
			continue
		}
		if v == '/' && i+1 < len(data) {
			next := data[i+1]
			if next == '*' {
				inComment = true
				i++
				continue
			}
			if next == '/' {
				inSingleLine = true
				i++
				continue
			}
		}
		out.WriteByte(v)
	}
	return out.String()
}

// ---- data json 解析（AList help.go） ----

var findDataRe = regexp.MustCompile(`data[:\s]+(\{[^}]+\})`)
var findKvRe = regexp.MustCompile(`'(.+?)':('?([^' },]*)'?)`)
var isNumberRe = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

func varRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`var ` + regexp.QuoteMeta(key) + `\s*=\s*['"]?(.+?)['"]?;`)
}

func findJSVarFunc(key, data string) string {
	if key == "sasign" {
		matches := varRe(key).FindAllStringSubmatch(data, -1)
		if len(matches) == 3 {
			return matches[1][1]
		}
		if len(matches) > 0 {
			return matches[0][1]
		}
		return ""
	}
	m := varRe(key).FindStringSubmatch(data)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func jsonToMap(data, html string) map[string]string {
	param := map[string]string{}
	for _, kv := range findKvRe.FindAllStringSubmatch(data, -1) {
		if len(kv) < 4 {
			continue
		}
		k := kv[1]
		raw := kv[2]
		v := kv[3]
		if v == "" || strings.ContainsRune(raw, '\'') || isNumberRe.MatchString(raw) {
			param[k] = v
		} else {
			param[k] = findJSVarFunc(v, html)
		}
	}
	return param
}

// htmlJsonToMap extracts the "data : { ... }" object and resolves JS variable
// references. Errors when no data parameter is found.
func htmlJsonToMap(html string) (map[string]string, error) {
	m := findDataRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, errors.New("页面中未找到 data 参数")
	}
	return jsonToMap(m[1], html), nil
}

// getJSFunctionByName locates "function name(...) {" and returns the complete
// balanced brace body.
func getJSFunctionByName(html, name string) (string, error) {
	re := regexp.MustCompile(`function\s+` + regexp.QuoteMeta(name) + `\s*\(`)
	m := re.FindStringIndex(html)
	if m == nil {
		return "", fmt.Errorf("页面中未找到 %s 函数", name)
	}
	start := strings.IndexByte(html[m[0]:], '{')
	if start < 0 {
		return "", fmt.Errorf("页面中未找到 %s 函数体", name)
	}
	start += m[0]
	depth := 0
	for i := start; i < len(html); i++ {
		switch html[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return html[m[0] : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("页面中 %s 函数不完整", name)
}

var fileIDRe = regexp.MustCompile(`'/ajaxm\.php\?file=(\d+)'`)
var downPageRe = regexp.MustCompile(`<iframe.*?src="(.+?)"`)
var nameFindRe = regexp.MustCompile(`<title>(.+?) - 蓝奏云</title>|id="filenajax">(.+?)</div>|var filename = '(.+?)';`)

// ajaxmResp is the parsed ajaxm.php response.
type ajaxmResp struct {
	Dom string
	URL string
	Inf string
}

func parseAjaxmResp(text string) (ajaxmResp, error) {
	var j map[string]any
	if err := json.Unmarshal([]byte(text), &j); err != nil {
		return ajaxmResp{}, fmt.Errorf("ajaxm 响应异常: %s", truncate(text, 120))
	}
	zt := numOf(j["zt"])
	if zt != 1 && zt != 2 && zt != 4 {
		msg := strOf(j["inf"])
		if msg == "" {
			msg = strOf(j["info"])
		}
		if msg == "" {
			msg = fmt.Sprintf("ajaxm 失败 zt=%d", zt)
		}
		return ajaxmResp{}, errors.New(msg)
	}
	return ajaxmResp{
		Dom: strOf(j["dom"]),
		URL: strOf(j["url"]),
		Inf: strOf(j["inf"]),
	}, nil
}

func numOf(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case json.Number:
		n, _ := t.Int64()
		return n
	}
	return 0
}

func strOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
