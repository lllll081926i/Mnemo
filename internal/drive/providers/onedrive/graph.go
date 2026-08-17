// Package onedrive implements the OneDrive provider (Microsoft Graph v1.0).
package onedrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const graphHost = "https://graph.microsoft.com/v1.0"

// RootID is the sentinel root folder id.
const RootID = "onedrive_root"

// Item is a raw Microsoft Graph driveItem.
type Item struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Size                 int64  `json:"size"`
	CreatedDateTime      string `json:"createdDateTime"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	DownloadURL          string `json:"@microsoft.graph.downloadUrl"`
	ContentDownloadURL   string `json:"@content.downloadUrl"`
	ParentReference      *struct {
		ID      string `json:"id"`
		DriveID string `json:"driveId"`
		Path    string `json:"path"`
	} `json:"parentReference"`
	Folder *struct {
		ChildCount int `json:"childCount"`
	} `json:"folder"`
	File *struct {
		MimeType string `json:"mimeType"`
		Hashes   *struct {
			SHA1Hash     string `json:"sha1Hash"`
			QuickXorHash string `json:"quickXorHash"`
		} `json:"hashes"`
	} `json:"file"`
	Image *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"image"`
	Video *struct {
		Width    int   `json:"width"`
		Height   int   `json:"height"`
		Duration int64 `json:"duration"`
	} `json:"video"`
	Thumbnails []struct {
		Small  *struct{ URL string } `json:"small"`
		Medium *struct{ URL string } `json:"medium"`
		Large  *struct{ URL string } `json:"large"`
	} `json:"thumbnails"`
}

// childrenResp is a paginated listing response.
type childrenResp struct {
	Value    []Item `json:"value"`
	NextLink string `json:"@odata.nextLink"`
}

// client wraps an authenticated Graph session.
type client struct {
	http  *netx.Client
	token string
}

func newClient(token string) *client {
	return &client{http: netx.NewClient(60 * time.Second), token: token}
}

func (c *client) headers(extra map[string]string) map[string]string {
	h := map[string]string{"Authorization": "Bearer " + c.token}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// trustedGraphURL guards against malicious pagination/monitor URLs.
func trustedGraphURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && (u.Port() == "" || u.Port() == "443") &&
		(host == "graph.microsoft.com" || strings.HasSuffix(host, ".sharepoint.com") ||
			strings.HasSuffix(host, ".sharepoint-df.com") || strings.HasSuffix(host, ".1drv.com"))
}

func (c *client) getJSON(ctx context.Context, pathOrURL string, out any) error {
	target := pathOrURL
	if !strings.HasPrefix(target, "https://") {
		target = graphHost + pathOrURL
	} else if !trustedGraphURL(target) {
		return errors.New("onedrive: invalid pagination url")
	}
	resp, err := c.http.Do(ctx, http.MethodGet, target, c.headers(nil), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return graphError(body, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func (c *client) jsonDo(ctx context.Context, method, path string, body any, out any) error {
	target := graphHost + path
	resp, err := c.http.Do(ctx, method, target, c.headers(map[string]string{"Content-Type": "application/json"}), netx.JSONBody(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return graphError(data, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// rawDo performs a request and returns the full response (for copy monitor).
func (c *client) rawDo(ctx context.Context, method, target string, body any, headers map[string]string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = netx.JSONBody(body)
	}
	return c.http.Do(ctx, method, target, c.headers(headers), reader)
}

func graphError(body []byte, status int) error {
	var g struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &g)
	msg := "onedrive: http " + fmt.Sprint(status)
	if g.Error != nil {
		if g.Error.Code != "" {
			msg += ": " + g.Error.Code
		}
		if g.Error.Message != "" {
			msg += ": " + g.Error.Message
		}
	}
	return errors.New(msg)
}

// listPath builds the children endpoint with $select/$expand.
func listPath(parentID string) string {
	const selectFields = "$select=id,name,size,file,folder,parentReference,createdDateTime,lastModifiedDateTime,fileSystemInfo,image,video,@microsoft.graph.downloadUrl,@content.downloadUrl"
	if parentID == "" || parentID == RootID {
		return "/me/drive/root/children?" + selectFields + "&$expand=thumbnails"
	}
	return "/me/drive/items/" + url.PathEscape(parentID) + "/children?" + selectFields + "&$expand=thumbnails"
}

// ListPage returns one page of a directory listing.
func (c *client) ListPage(ctx context.Context, parentID, nextLink string) ([]Item, string, error) {
	target := nextLink
	if target == "" {
		target = listPath(parentID)
	}
	var resp childrenResp
	if err := c.getJSON(ctx, target, &resp); err != nil {
		return nil, "", err
	}
	if resp.NextLink != "" && !trustedGraphURL(resp.NextLink) {
		return nil, "", errors.New("onedrive: invalid next link")
	}
	return resp.Value, resp.NextLink, nil
}

// List fetches all pages of a directory.
func (c *client) List(ctx context.Context, parentID string) ([]Item, error) {
	var out []Item
	seen := map[string]bool{}
	next := ""
	for {
		items, link, err := c.ListPage(ctx, parentID, next)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if link == "" {
			break
		}
		if seen[link] {
			return nil, errors.New("onedrive: duplicate paging link")
		}
		seen[link] = true
		next = link
	}
	return out, nil
}

// Detail returns one item (or nil).
func (c *client) Detail(ctx context.Context, fileID string) (*Item, error) {
	path := "/me/drive/root?$expand=thumbnails"
	if fileID != "" && fileID != RootID {
		path = "/me/drive/items/" + url.PathEscape(fileID) + "?$expand=thumbnails"
	}
	var item Item
	if err := c.getJSON(ctx, path, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// Search uses the Graph search endpoint.
func (c *client) Search(ctx context.Context, keyword string) ([]Item, error) {
	var out []Item
	// Graph parses the search argument as an OData string literal. Escape a
	// literal apostrophe first, then escape the complete value as a path
	// segment so spaces remain spaces instead of becoming '+' in the URL path.
	odataKeyword := strings.ReplaceAll(keyword, "'", "''")
	target := "/me/drive/root/search(q='" + url.PathEscape(odataKeyword) + "')"
	seen := map[string]bool{}
	for target != "" {
		if seen[target] {
			break
		}
		seen[target] = true
		var resp childrenResp
		if err := c.getJSON(ctx, target, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Value...)
		target = resp.NextLink
		// getJSON validates and accepts both Graph-relative and trusted
		// absolute next links. SharePoint-backed drives may return a host other
		// than graph.microsoft.com, so do not discard those cursors here.
	}
	return out, nil
}

// Mkdir creates a folder (conflictBehavior rename).
func (c *client) Mkdir(ctx context.Context, parentID, name string) (*drive.MkdirResult, error) {
	parent := "root"
	if parentID != "" && parentID != RootID {
		parent = "items/" + url.PathEscape(parentID)
	}
	var item Item
	err := c.jsonDo(ctx, http.MethodPost, "/me/drive/"+parent+"/children", map[string]any{
		"name":                              name,
		"folder":                            map[string]any{},
		"@microsoft.graph.conflictBehavior": "rename",
	}, &item)
	if err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	return &drive.MkdirResult{FileID: item.ID}, nil
}

// Rename patches the name.
func (c *client) Rename(ctx context.Context, fileID, name string) (*Item, error) {
	var item Item
	err := c.jsonDo(ctx, http.MethodPatch, "/me/drive/items/"+url.PathEscape(fileID), map[string]any{"name": name}, &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Delete permanently removes an item (OneDrive trash is not exposed here).
func (c *client) Delete(ctx context.Context, fileID string) error {
	return c.jsonDo(ctx, http.MethodDelete, "/me/drive/items/"+url.PathEscape(fileID), nil, nil)
}

func parentRef(targetParentID string) map[string]any {
	if targetParentID == "" || targetParentID == RootID {
		return map[string]any{"path": "/drive/root:"}
	}
	return map[string]any{"id": targetParentID}
}

// Move patches parentReference.
func (c *client) Move(ctx context.Context, fileID, targetParentID string) error {
	return c.jsonDo(ctx, http.MethodPatch, "/me/drive/items/"+url.PathEscape(fileID),
		map[string]any{"parentReference": parentRef(targetParentID)}, nil)
}

// Copy initiates an async copy and polls the monitor URL until completion.
func (c *client) Copy(ctx context.Context, fileID, targetParentID, name string) error {
	resp, err := c.rawDo(ctx, http.MethodPost,
		graphHost+"/me/drive/items/"+url.PathEscape(fileID)+"/copy",
		map[string]any{"parentReference": parentRef(targetParentID), "name": name},
		map[string]string{"Content-Type": "application/json", "Prefer": "respond-async"})
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return graphError(body, resp.StatusCode)
	}
	if resp.StatusCode != 202 {
		return nil // done synchronously
	}
	monitorURL := resp.Header.Get("Location")
	if monitorURL == "" {
		return errors.New("onedrive: copy monitor url missing")
	}
	if !trustedGraphURL(monitorURL) {
		return errors.New("onedrive: invalid copy monitor url")
	}
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		status, err := c.pollCopy(ctx, monitorURL)
		if err != nil {
			return err
		}
		if status == "" {
			return nil
		}
	}
	return errors.New("onedrive: copy timeout")
}

func (c *client) pollCopy(ctx context.Context, monitorURL string) (string, error) {
	resp, err := c.http.Do(ctx, http.MethodGet, monitorURL, c.headers(nil), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 303 {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		var g struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &g)
		if g.Error != nil {
			return "", errors.New(g.Error.Message)
		}
		return "", fmt.Errorf("onedrive: copy poll http %d", resp.StatusCode)
	}
	var st struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(body, &st)
	switch strings.ToLower(st.Status) {
	case "", "completed":
		return "", nil
	case "failed", "deletefailed":
		return "", errors.New("onedrive: copy failed")
	}
	return "running", nil
}

// CreateLink creates an anonymous view link (optionally with password/expiration).
func (c *client) CreateLink(ctx context.Context, fileID, expiration, password string) (*model.ShareItem, error) {
	body := map[string]any{"type": "view", "scope": "anonymous"}
	if password != "" {
		body["password"] = password
	}
	if expiration != "" {
		body["expirationDateTime"] = expiration
	}
	var perm struct {
		ID                 string `json:"id"`
		ExpirationDateTime string `json:"expirationDateTime"`
		Link               *struct {
			Type   string `json:"type"`
			Scope  string `json:"scope"`
			WebURL string `json:"webUrl"`
		} `json:"link"`
	}
	if err := c.jsonDo(ctx, http.MethodPost, "/me/drive/items/"+url.PathEscape(fileID)+"/createLink", body, &perm); err != nil {
		return nil, err
	}
	if perm.Link == nil || perm.Link.WebURL == "" {
		return nil, errors.New("onedrive: no share url returned")
	}
	return &model.ShareItem{
		ShareID:     perm.ID,
		ShareURL:    perm.Link.WebURL,
		SharePolicy: perm.Link.Scope,
		ShareName:   "OneDrive 分享链接",
		Expiration:  perm.ExpirationDateTime,
		SharePwd:    password,
	}, nil
}

// DownloadURL returns the item's download url (or the content endpoint).
func (c *client) DownloadURL(item *Item) string {
	if item.DownloadURL != "" {
		return item.DownloadURL
	}
	if item.ContentDownloadURL != "" {
		return item.ContentDownloadURL
	}
	return graphHost + "/me/drive/items/" + url.PathEscape(item.ID) + "/content"
}

// UploadSessionItem is a createUploadSession response.
type UploadSessionItem struct {
	UploadURL          string `json:"uploadUrl"`
	ExpirationDateTime string `json:"expirationDateTime"`
}

// CreateUploadSession starts a resumable upload session.
func (c *client) CreateUploadSession(ctx context.Context, parentID, name, conflictBehavior string) (*UploadSessionItem, error) {
	parent := "root"
	if parentID != "" && parentID != RootID {
		parent = "items/" + url.PathEscape(parentID)
	}
	var out UploadSessionItem
	if conflictBehavior == "" {
		conflictBehavior = "replace"
	}
	if err := c.jsonDo(ctx, http.MethodPost,
		"/me/drive/"+parent+":/"+url.PathEscape(name)+":/createUploadSession",
		map[string]any{
			"item": map[string]any{
				"@microsoft.graph.conflictBehavior": conflictBehavior,
				"name":                              name,
			},
		}, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.UploadURL) == "" {
		return nil, errors.New("onedrive: upload session url missing")
	}
	return &out, nil
}

// mapItem converts a Graph item to the unified file model.
func mapItem(item *Item, driveID, parentID string) model.File {
	isDir := item.Folder != nil
	name := item.Name
	var mimeType string
	if item.File != nil {
		mimeType = item.File.MimeType
	}
	ext := ""
	if !isDir {
		ext = driveutil.Ext(name)
	}
	timeUnix := int64(0)
	if t := item.LastModifiedDateTime; t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			timeUnix = parsed.Unix()
		}
	}
	if timeUnix == 0 {
		if parsed, err := time.Parse(time.RFC3339, item.CreatedDateTime); err == nil {
			timeUnix = parsed.Unix()
		}
	}
	size := item.Size
	hash := ""
	hashName := ""
	if item.File != nil && item.File.Hashes != nil {
		if item.File.Hashes.SHA1Hash != "" {
			hash, hashName = item.File.Hashes.SHA1Hash, "sha1"
		} else if item.File.Hashes.QuickXorHash != "" {
			hash, hashName = item.File.Hashes.QuickXorHash, "quickxorhash"
		}
	}
	f := driveutil.NewFile(driveID, item.ID, parentID, name, isDir, size, timeUnix)
	f.MimeType = mimeType
	f.MimeExtension = ext
	f.ContentHash = hash
	f.ContentHashName = hashName
	f.FileCount = 0
	if item.Folder != nil {
		f.FileCount = int64(item.Folder.ChildCount)
	}
	if !isDir && len(item.Thumbnails) > 0 {
		t := item.Thumbnails[0]
		switch {
		case t.Medium != nil:
			f.Thumbnail = t.Medium.URL
		case t.Large != nil:
			f.Thumbnail = t.Large.URL
		case t.Small != nil:
			f.Thumbnail = t.Small.URL
		}
	}
	if item.Image != nil {
		f.MediaWidth, f.MediaHeight = item.Image.Width, item.Image.Height
	}
	if item.Video != nil {
		f.MediaWidth, f.MediaHeight = item.Video.Width, item.Video.Height
		if item.Video.Duration > 0 {
			f.MediaDuration = fmt.Sprint(item.Video.Duration / 1000)
		}
	}
	// description encodes parent/drive/download hints
	parts := []string{}
	if item.ParentReference != nil {
		if item.ParentReference.ID != "" {
			parts = append(parts, "onedrive_parent:"+item.ParentReference.ID)
		}
		if item.ParentReference.Path != "" {
			parts = append(parts, "onedrive_path:"+url.QueryEscape(item.ParentReference.Path))
		}
		if item.ParentReference.DriveID != "" {
			parts = append(parts, "onedrive_drive:"+item.ParentReference.DriveID)
		}
	}
	if item.DownloadURL != "" {
		parts = append(parts, "onedrive_download:"+url.QueryEscape(item.DownloadURL))
	}
	f.Description = strings.Join(parts, ";")
	return f
}
