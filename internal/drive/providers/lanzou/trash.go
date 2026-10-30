package lanzou

import (
	"context"
	"errors"
	"fmt"
	stdhtml "html"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

func recycleFormToken(page string) string {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}
	var token string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "input" {
			name, value := "", ""
			for _, attr := range node.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
				if attr.Key == "value" {
					value = attr.Val
				}
			}
			if strings.EqualFold(name, "formhash") {
				token = value
			}
		}
		for child := node.FirstChild; child != nil && token == ""; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return token
}

func (d *Driver) recycleRequest(ctx context.Context, c drive.Context, method string, query, form url.Values) (string, error) {
	cookie, _, _, baseURL := sessionOf(c)
	if cookie == "" {
		return "", drive.ErrUnauthorized
	}
	target := strings.TrimSuffix(baseURL, "/") + "/mydisk.php?" + query.Encode()
	var body []byte
	headers := map[string]string{"referer": strings.TrimSuffix(baseURL, "/") + "/mydisk.php", "user-agent": LANZOU_DEFAULT.UserAgent}
	if method == http.MethodPost {
		body = []byte(form.Encode())
		headers["content-type"] = "application/x-www-form-urlencoded"
	}
	result, err := fetchText(ctx, method, target, headers, body, cookie, false)
	if err != nil {
		return "", err
	}
	if result.status < 200 || result.status >= 300 {
		return "", fmt.Errorf("蓝奏回收站 HTTP %d", result.status)
	}
	if strings.Contains(result.text, "网盘用户登录") || strings.Contains(result.text, "用户登录</title>") {
		return "", drive.AuthExpired("蓝奏云网页登录已失效，请重新登录")
	}
	return result.text, nil
}

func (d *Driver) ListTrash(ctx context.Context, c drive.Context, _ *drive.ListOptions) ([]model.File, error) {
	page, err := d.recycleRequest(ctx, c, http.MethodGet, url.Values{"item": {"recycle"}, "action": {"files"}}, nil)
	if err != nil {
		return nil, err
	}
	items, err := parseLanzouRecycle(page, c.DriveID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 && recycleFormToken(page) == "" && !strings.Contains(page, "回收站") {
		return nil, errors.New("蓝奏云回收站页面未返回有效内容，请刷新重试或前往官网检查")
	}
	return items, nil
}

func parseLanzouRecycle(page, driveID string) ([]model.File, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil, err
	}
	items := make([]model.File, 0)
	seen := make(map[string]bool)
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			id, name, isDir := recycleRow(node)
			if id != "" && name != "" {
				prefix := "file:"
				if isDir {
					prefix = "folder:"
				}
				key := prefix + id
				if !seen[key] {
					seen[key] = true
					items = append(items, driveutil.NewFile(driveID, key, "trash", name, isDir, 0, 0))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return items, nil
}

func recycleRow(row *html.Node) (id, name string, isDir bool) {
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			attrs := make(map[string]string, len(node.Attr))
			for _, attr := range node.Attr {
				attrs[attr.Key] = attr.Val
			}
			if node.Data == "input" {
				if strings.HasPrefix(attrs["name"], "fl_sel_ids") || strings.HasPrefix(attrs["name"], "fd_sel_ids") {
					id = attrs["value"]
					isDir = strings.HasPrefix(attrs["name"], "fd_sel_ids")
				}
			}
			if node.Data == "a" {
				if id == "" {
					if link, err := url.Parse(attrs["href"]); err == nil && link.Query().Get("folder_id") != "" {
						id = link.Query().Get("folder_id")
						isDir = true
					}
				}
				if name == "" {
					name = strings.TrimSpace(recycleText(node))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(row)
	return id, name, isDir
}

func recycleText(node *html.Node) string {
	if node.Type == html.TextNode {
		return stdhtml.UnescapeString(node.Data)
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(recycleText(child))
	}
	return text.String()
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	var completed []string
	var failures []error
	for _, id := range fileIDs {
		action, key, value := "file_restore", "file_id", strings.TrimPrefix(id, "file:")
		if strings.HasPrefix(id, "folder:") {
			action, key, value = "folder_restore", "folder_id", strings.TrimPrefix(id, "folder:")
		}
		if value == "" || strings.ContainsAny(value, "?&/") {
			failures = append(failures, fmt.Errorf("%s: 回收站文件 ID 无效", id))
			continue
		}
		page, err := d.recycleRequest(ctx, c, http.MethodGet, url.Values{"item": {"recycle"}, "action": {action}, key: {value}}, nil)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		token := recycleFormToken(page)
		if token == "" {
			failures = append(failures, fmt.Errorf("%s: 无法获取蓝奏回收站表单", id))
			continue
		}
		response, err := d.recycleRequest(ctx, c, http.MethodPost, url.Values{"item": {"recycle"}}, url.Values{"action": {action}, "task": {action}, key: {value}, "formhash": {token}})
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", id, err))
			continue
		}
		if !strings.Contains(response, "恢复成功") {
			failures = append(failures, fmt.Errorf("%s: 蓝奏云未确认恢复成功，请检查官网回收站", id))
			continue
		}
		completed = append(completed, id)
	}
	return completed, errors.Join(failures...)
}
