// Package webdav implements a minimal RFC 4918 WebDAV client used by the
// webdav drive provider. Pure Go, no external dependency. PROPFIND responses
// are parsed namespace-agnostically with an XML token walk.
package webdav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// Client is a WebDAV connection bound to an endpoint + credentials.
type Client struct {
	HTTP     *http.Client
	Endpoint string
	Username string
	Password string
	UA       string
	base     *url.URL
	rootPath string
}

// New builds a client from a connection config.
func New(conn *model.ConnConfig, timeout time.Duration) (*Client, error) {
	if conn == nil || conn.Endpoint == "" {
		return nil, errors.New("webdav: missing endpoint")
	}
	rawEndpoint := strings.TrimSpace(conn.Endpoint)
	if !strings.Contains(rawEndpoint, "://") {
		rawEndpoint = "https://" + rawEndpoint
	}
	base, err := url.Parse(rawEndpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, errors.New("webdav: endpoint must be a valid http(s) URL")
	}
	if !strings.EqualFold(base.Scheme, "http") && !strings.EqualFold(base.Scheme, "https") {
		return nil, errors.New("webdav: endpoint scheme must be http or https")
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("webdav: endpoint must not contain credentials, query, or fragment")
	}
	basePath, err := normalizeDAVPath(base.Path)
	if err != nil {
		return nil, fmt.Errorf("webdav: invalid endpoint path: %w", err)
	}
	base.Path = basePath
	base.RawPath = ""
	configuredRootPath := strings.TrimSpace(conn.RootPath)
	if configuredRootPath == "" {
		// Older Mnemo-Go builds used BasePath for both mounted providers.
		// Keep those persisted WebDAV accounts on the same visible subtree.
		configuredRootPath = conn.BasePath
	}
	rootPath, err := normalizeDAVPath(configuredRootPath)
	if err != nil {
		return nil, fmt.Errorf("webdav: invalid root path: %w", err)
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		HTTP:     &http.Client{Timeout: timeout},
		Endpoint: strings.TrimRight(base.String(), "/"),
		Username: conn.Username,
		Password: conn.Password,
		UA:       netx.DefaultUA,
		base:     base,
		rootPath: rootPath,
	}, nil
}

func (c *Client) newReq(ctx context.Context, method, href string, body io.Reader, headers map[string]string) (*http.Request, error) {
	rawURL, err := c.resolve(href)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UA)
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func (c *Client) resolve(href string) (string, error) {
	if c == nil || c.base == nil {
		return "", errors.New("webdav: client is not initialized")
	}
	raw := strings.TrimSpace(href)
	var resource string
	if raw != "" {
		if u, err := url.Parse(raw); err == nil && u.IsAbs() {
			if !sameOrigin(c.base, u) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return "", errors.New("webdav: resource href points outside the configured endpoint")
			}
			resource = u.Path
		} else if strings.HasPrefix(raw, "//") {
			return "", errors.New("webdav: protocol-relative resource href is not allowed")
		} else {
			resource = raw
		}
	}
	resource, err := normalizeDAVPath(resource)
	if err != nil {
		return "", err
	}
	resource = c.withEndpointPath(resource)
	target := *c.base
	target.Path = resource
	encoded, err := escapeDAVPath(resource)
	if err != nil {
		return "", err
	}
	target.RawPath = encoded
	target.RawQuery = ""
	target.Fragment = ""
	return target.String(), nil
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && a.Port() == b.Port()
}

func normalizeDAVPath(raw string) (string, error) {
	if raw == "" || raw == "root" || raw == "/" {
		return "/", nil
	}
	if strings.IndexByte(raw, 0) >= 0 || strings.ContainsRune(raw, '\\') {
		return "", errors.New("webdav: invalid path")
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	trailing := strings.HasSuffix(raw, "/")
	parts := make([]string, 0, strings.Count(raw, "/"))
	for _, part := range strings.Split(raw, "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", errors.New("webdav: path traversal is not allowed")
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "/", nil
	}
	clean := "/" + strings.Join(parts, "/")
	if trailing {
		clean += "/"
	}
	return clean, nil
}

// escapeDAVPath encodes each path segment independently. Encoding the whole
// path would turn separators into data; leaving it unescaped makes names such
// as "#" and "?" become URL fragments or queries.
func escapeDAVPath(raw string) (string, error) {
	normalized, err := normalizeDAVPath(raw)
	if err != nil {
		return "", err
	}
	if normalized == "/" {
		return "/", nil
	}
	trailing := strings.HasSuffix(normalized, "/")
	parts := strings.Split(strings.Trim(normalized, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	encoded := "/" + strings.Join(parts, "/")
	if trailing {
		encoded += "/"
	}
	return encoded, nil
}

func (c *Client) endpointPath() string {
	if c == nil || c.base == nil || c.base.Path == "" {
		if c == nil || c.rootPath == "" || c.rootPath == "/" {
			return "/"
		}
		return strings.TrimRight(c.rootPath, "/")
	}
	base := strings.TrimRight(c.base.Path, "/")
	if c.rootPath == "" || c.rootPath == "/" {
		return base
	}
	return base + "/" + strings.Trim(c.rootPath, "/")
}

func (c *Client) withEndpointPath(resource string) string {
	base := c.endpointPath()
	if base == "/" {
		return strings.TrimRight(resource, "/")
	}
	resource = strings.TrimRight(resource, "/")
	if resource == "" {
		return base
	}
	if resource == base || strings.HasPrefix(resource, base+"/") {
		return resource
	}
	return base + resource
}

// logicalPath converts a server href to the provider's endpoint-relative id.
// WebDAV servers may return either an absolute URL or a path including the
// configured collection prefix.
func (c *Client) logicalPath(href string) (string, error) {
	raw := strings.TrimSpace(href)
	if raw == "" {
		return "/", nil
	}
	if u, err := url.Parse(raw); err == nil && u.IsAbs() {
		if !sameOrigin(c.base, u) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", errors.New("webdav: resource href points outside the configured endpoint")
		}
		raw = u.EscapedPath()
	} else if strings.HasPrefix(raw, "//") {
		return "", errors.New("webdav: protocol-relative resource href is not allowed")
	}
	var err error
	raw, err = unescapeDAVPath(raw)
	if err != nil {
		return "", fmt.Errorf("webdav: invalid resource href: %w", err)
	}
	p, err := normalizeDAVPath(raw)
	if err != nil {
		return "", err
	}
	base := c.endpointPath()
	if base != "/" {
		if p == base {
			return "/", nil
		}
		if strings.HasPrefix(p, base+"/") {
			p = strings.TrimPrefix(p, base)
		}
	}
	if p == "" {
		return "/", nil
	}
	return p, nil
}

func unescapeDAVPath(raw string) (string, error) {
	parts := strings.Split(raw, "/")
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return "", err
		}
		parts[i] = decoded
	}
	return strings.Join(parts, "/"), nil
}

// Entry is a normalized directory listing entry.
type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64
	Modified time.Time
}

// davEntry is one PROPFIND resource parsed namespace-agnostically.
type davEntry struct {
	Href  string
	Props map[string]string
}

func (e davEntry) davValue(names ...string) string {
	for _, want := range names {
		if v, ok := e.Props[want]; ok {
			return v
		}
	}
	return ""
}

func (e davEntry) davBool(names ...string) bool {
	v := strings.TrimSpace(e.davValue(names...))
	return v == "1" || strings.EqualFold(v, "true")
}

// parsePropfind walks a multistatus document namespace-agnostically.
func parsePropfind(data []byte) ([]davEntry, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out []davEntry
	var cur *davEntry
	var inProp bool
	var propName, propText string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "response":
				cur = &davEntry{Props: map[string]string{}}
			case "href":
				propName = ""
				propText = ""
			case "prop":
				inProp = true
			default:
				if inProp {
					propName = t.Name.Local
					propText = ""
				}
			}
		case xml.CharData:
			propText += string(t)
		case xml.EndElement:
			switch t.Name.Local {
			case "href":
				if cur != nil {
					cur.Href = propText
				}
				propText = ""
			case "prop":
				inProp = false
			case "response":
				if cur != nil {
					out = append(out, *cur)
					cur = nil
				}
			default:
				if inProp && propName != "" {
					if cur != nil {
						cur.Props[propName] = propText
					}
					propName = ""
					propText = ""
				}
			}
		}
	}
	return out, nil
}

// List PROPFINDs a directory (depth 1) and returns its entries.
func (c *Client) List(ctx context.Context, href string) ([]Entry, error) {
	req, err := c.newReq(ctx, "PROPFIND", href, strings.NewReader(propfindAllBody), map[string]string{
		"Depth":        "1",
		"Content-Type": "application/xml",
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("webdav: PROPFIND %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	entries, err := parsePropfind(body)
	if err != nil {
		return nil, fmt.Errorf("webdav: PROPFIND decode: %w", err)
	}
	base, err := c.logicalPath(href)
	if err != nil {
		return nil, err
	}
	base = strings.TrimRight(base, "/") + "/"
	var out []Entry
	for _, r := range entries {
		p, err := c.logicalPath(r.Href)
		if err != nil {
			return nil, err
		}
		if strings.TrimRight(p, "/") == strings.TrimRight(base, "/") {
			continue
		}
		isDir := r.davBool("resourcetype", "collection") || strings.HasSuffix(p, "/")
		name := path.Base(strings.TrimRight(p, "/"))
		if name == "" || name == "." || name == "/" {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(r.davValue("getcontentlength")), 10, 64)
		var mod time.Time
		if s := strings.TrimSpace(r.davValue("getlastmodified")); s != "" {
			mod, _ = time.Parse(http.TimeFormat, s)
		}
		out = append(out, Entry{Name: name, Path: p, IsDir: isDir, Size: size, Modified: mod})
	}
	return out, nil
}

// Stat PROPFINDs a single resource (depth 0).
func (c *Client) Stat(ctx context.Context, href string) (*Entry, error) {
	req, err := c.newReq(ctx, "PROPFIND", href, strings.NewReader(propfindAllBody), map[string]string{
		"Depth":        "0",
		"Content-Type": "application/xml",
	})
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("webdav: not found")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("webdav: PROPFIND %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	entries, err := parsePropfind(body)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("webdav: empty PROPFIND response")
	}
	r := entries[0]
	p, err := c.logicalPath(r.Href)
	if err != nil {
		return nil, err
	}
	isDir := r.davBool("resourcetype", "collection")
	size, _ := strconv.ParseInt(strings.TrimSpace(r.davValue("getcontentlength")), 10, 64)
	var mod time.Time
	if s := strings.TrimSpace(r.davValue("getlastmodified")); s != "" {
		mod, _ = time.Parse(http.TimeFormat, s)
	}
	return &Entry{Name: path.Base(strings.TrimRight(p, "/")), Path: p, IsDir: isDir, Size: size, Modified: mod}, nil
}

// Mkcol creates a directory.
func (c *Client) Mkcol(ctx context.Context, href string) error {
	req, err := c.newReq(ctx, "MKCOL", href, nil, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != 405 && resp.StatusCode != 301 {
		return fmt.Errorf("webdav: MKCOL %d", resp.StatusCode)
	}
	return nil
}

// Delete removes a resource.
func (c *Client) Delete(ctx context.Context, href string) error {
	req, err := c.newReq(ctx, "DELETE", href, nil, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != 404 {
		return fmt.Errorf("webdav: DELETE %d", resp.StatusCode)
	}
	return nil
}

// Move renames/moves a resource.
func (c *Client) Move(ctx context.Context, from, to string) error {
	destination, err := c.resolve(to)
	if err != nil {
		return err
	}
	req, err := c.newReq(ctx, "MOVE", from, nil, map[string]string{
		"Destination": destination,
		"Overwrite":   "T",
	})
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav: MOVE %d", resp.StatusCode)
	}
	return nil
}

// Copy copies a resource to a destination.
func (c *Client) Copy(ctx context.Context, from, to string) error {
	destination, err := c.resolve(to)
	if err != nil {
		return err
	}
	req, err := c.newReq(ctx, "COPY", from, nil, map[string]string{
		"Destination": destination,
		"Overwrite":   "T",
	})
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav: COPY %d", resp.StatusCode)
	}
	return nil
}

// Put uploads a body to href.
func (c *Client) Put(ctx context.Context, href string, body io.Reader, size int64) error {
	req, err := c.newReq(ctx, "PUT", href, body, map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if err != nil {
		return err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav: PUT %d", resp.StatusCode)
	}
	return nil
}

// DownloadURL returns the direct URL for a resource.
func (c *Client) DownloadURL(href string) (string, error) { return c.resolve(href) }

const propfindAllBody = `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`
