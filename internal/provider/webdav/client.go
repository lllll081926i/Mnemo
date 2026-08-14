// Package webdav implements a minimal RFC 4918 WebDAV client used by the
// webdav drive provider. Pure Go, no external dependency.
package webdav

import (
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
	Endpoint string // base endpoint, e.g. https://dav.example.com
	Username string
	Password string
	UA       string
}

// New builds a client from a connection config.
func New(conn *model.ConnConfig, timeout time.Duration) (*Client, error) {
	if conn == nil || conn.Endpoint == "" {
		return nil, errors.New("webdav: missing endpoint")
	}
	endpoint := strings.TrimRight(conn.Endpoint, "/")
	return &Client{
		HTTP:     &http.Client{Timeout: timeout},
		Endpoint: endpoint,
		Username: conn.Username,
		Password: conn.Password,
		UA:       netx.DefaultUA,
	}, nil
}

// BasicAuth applies basic auth header when credentials are set.
func (c *Client) BasicAuth(req *http.Request) {
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
}

func (c *Client) newReq(ctx context.Context, method, href string, body io.Reader, headers map[string]string) (*http.Request, error) {
	rawURL := c.resolve(href)
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UA)
	c.BasicAuth(req)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// resolve joins the endpoint with an href (absolute or relative).
func (c *Client) resolve(href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if href == "" {
		href = "/"
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return c.Endpoint + href
}

// Prop is one DAV property value.
type Prop struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

// MultiStatus is a parsed PROPFIND response.
type MultiStatus struct {
	Responses []Response `xml:"response"`
}

// Response is one PROPFIND resource entry.
type Response struct {
	Href  string `xml:"href"`
	Props []Prop `xml:"propstat>prop"`
}

// propValue extracts a property by local name.
func (r Response) propValue(names ...string) string {
	for _, p := range r.Props {
		for _, want := range names {
			if strings.EqualFold(p.XMLName.Local, want) {
				return p.Value
			}
		}
	}
	return ""
}

// propBool extracts a property as boolean.
func (r Response) propBool(names ...string) bool {
	v := strings.TrimSpace(r.propValue(names...))
	return v == "1" || strings.EqualFold(v, "true")
}

// Entry is a normalized directory listing entry.
type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64
	Modified time.Time
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
	var ms MultiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("webdav: PROPFIND decode: %w", err)
	}
	base := href
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimRight(base, "/") + "/"
	var out []Entry
	for _, r := range ms.Responses {
		p := r.Href
		if u, err := url.Parse(p); err == nil && u.Path != "" {
			p = u.Path
		}
		decoded, err := url.PathUnescape(p)
		if err == nil {
			p = decoded
		}
		// skip the queried directory itself
		if strings.TrimRight(p, "/") == strings.TrimRight(base, "/") {
			continue
		}
		isDir := r.propBool("resourcetype", "directory") || strings.HasSuffix(p, "/")
		name := path.Base(strings.TrimRight(p, "/"))
		if name == "" || name == "." || name == "/" {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(r.propValue("getcontentlength")), 10, 64)
		var mod time.Time
		if s := strings.TrimSpace(r.propValue("getlastmodified")); s != "" {
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
	var ms MultiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}
	if len(ms.Responses) == 0 {
		return nil, errors.New("webdav: empty PROPFIND response")
	}
	r := ms.Responses[0]
	p := r.Href
	if u, err := url.Parse(p); err == nil && u.Path != "" {
		p = u.Path
	}
	isDir := r.propBool("resourcetype", "directory")
	size, _ := strconv.ParseInt(strings.TrimSpace(r.propValue("getcontentlength")), 10, 64)
	return &Entry{Name: path.Base(strings.TrimRight(p, "/")), Path: p, IsDir: isDir, Size: size}, nil
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

// Move renames/moves a resource (destination may be absolute or relative).
func (c *Client) Move(ctx context.Context, from, to string) error {
	dest := c.resolve(to)
	req, err := c.newReq(ctx, "MOVE", from, nil, map[string]string{
		"Destination": dest,
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
	req, err := c.newReq(ctx, "COPY", from, nil, map[string]string{
		"Destination": c.resolve(to),
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
	if size > 0 {
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
func (c *Client) DownloadURL(href string) string { return c.resolve(href) }

const propfindAllBody = `<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`
