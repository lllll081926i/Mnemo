package onedrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"mnemo-go/internal/model"
)

// Graph simple upload limit (4 MiB).
const smallUploadLimit = 4 * 1024 * 1024

// Upload session chunk size: multiple of 320 KiB per Graph rules.
const sessionChunkSize = 10 * 1024 * 1024

func encodePathSegment(v string) string {
	return url.PathEscape(v)
}

// smallUploadPath builds the PUT content path for small files.
func smallUploadPath(parentID, fileName string) string {
	name := encodePathSegment(fileName)
	if parentID == "" || parentID == RootID {
		return "/me/drive/root:/" + name + ":/content"
	}
	return "/me/drive/items/" + url.PathEscape(parentID) + ":/" + name + ":/content"
}

// rawPut performs a PUT with an auth header.
func (c *client) rawPut(ctx context.Context, target string, body io.Reader) error {
	resp, err := c.http.Do(ctx, http.MethodPut, target, c.headers(map[string]string{"Content-Type": "application/octet-stream"}), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return graphError(data, resp.StatusCode)
	}
	return nil
}

// sessionUpload uploads a file via a Graph upload session with chunked PUTs,
// reporting progress through the upload UI model.
func (c *client) sessionUpload(ctx context.Context, f *os.File, parentID, name string, ui *model.UploadingUI) error {
	sess, err := c.CreateUploadSession(ctx, parentID, name)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	var pos int64
	buf := make([]byte, sessionChunkSize)
	for pos < size {
		n, err := f.ReadAt(buf, pos)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		chunk := buf[:n]
		end := pos + int64(n) - 1
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, sess.UploadURL, io.NopCloser(readBytes{chunk}))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Range", "bytes "+strconv.FormatInt(pos, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
		req.Header.Set("Content-Length", strconv.Itoa(n))
		resp, err := c.http.HTTP.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 && resp.StatusCode != 201 && resp.StatusCode != 200 {
			// 416 → complete already
			if resp.StatusCode == 416 {
				return nil
			}
			return graphError(body, resp.StatusCode)
		}
		pos += int64(n)
		if ui != nil && ui.Upload.DownSize >= 0 {
			ui.Upload.DownSize = pos
			if size > 0 {
				ui.Upload.DownProcess = int(pos * 100 / size)
			}
		}
	}
	return nil
}

// readBytes adapts a byte slice to io.Reader.
type readBytes struct{ b []byte }

func (r readBytes) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

var _ = json.Marshal
var _ = fmt.Sprint
var _ = errors.New
