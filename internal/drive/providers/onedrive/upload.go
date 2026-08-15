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

	"mnemo-go/internal/drive"
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
func (c *client) sessionUpload(ctx context.Context, dc drive.Context, f *os.File, parentID, name string, ui *model.UploadingUI) error {
	sess, err := c.CreateUploadSession(ctx, parentID, name)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	// Resume: query session position from server to skip already-uploaded bytes
	sessionKey := drive.UploadSessionKey(dc.UserID, dc.DriveID, parentID, name, size)
	var pos int64
	if qPos, qFile, completed := querySessionPosition(ctx, sess.UploadURL, c); completed {
		// Session already complete (all parts uploaded, just crashed before clear)
		if ui != nil {
			ui.Upload.FileID = qFile
		}
		drive.ClearUploadSession(sessionKey)
		return nil
	} else if qPos > 0 && qPos <= size {
		pos = qPos
	}

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
			// 416 → already uploaded; query position to advance
			if resp.StatusCode == 416 {
				if qPos, qFile, completed := querySessionPosition(ctx, sess.UploadURL, c); completed {
					if ui != nil {
						ui.Upload.FileID = qFile
					}
					drive.ClearUploadSession(sessionKey)
					return nil
				} else if qPos > pos && qPos <= size {
					pos = qPos
					continue
				}
				return nil
			}
			return graphError(body, resp.StatusCode)
		}
		pos += int64(n)
		// Persist progress incrementally (byte offset as single-element part list)
		_ = drive.SaveUploadSession(sessionKey, []int{int(pos)})
		if ui != nil && ui.Upload.DownSize >= 0 {
			ui.Upload.DownSize = pos
			if size > 0 {
				ui.Upload.DownProcess = int(pos * 100 / size)
			}
		}
	}
	drive.ClearUploadSession(sessionKey)
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

// querySessionPosition queries the Graph upload session URL to discover the
// next expected byte range (resumable upload). Returns position, fileId (when
// the session is already complete), and a completed flag.
func querySessionPosition(ctx context.Context, uploadURL string, c *client) (pos int64, fileID string, completed bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uploadURL, nil)
	if err != nil {
		return 0, "", false
	}
	resp, err := c.http.HTTP.Do(req)
	if err != nil {
		return 0, "", false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	// 200/201/202 with a file id = session complete
	if resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 202 {
		var done struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &done) == nil && done.ID != "" {
			return 0, done.ID, true
		}
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return 0, "", false
	}
	var data struct {
		NextExpectedRanges []string `json:"nextExpectedRanges"`
	}
	if json.Unmarshal(body, &data) != nil {
		return 0, "", false
	}
	if len(data.NextExpectedRanges) == 0 {
		return 0, "", false
	}
	first := data.NextExpectedRanges[0]
	if idx := indexByte(first, '-'); idx > 0 {
		first = first[:idx]
	}
	var p int64
	fmt.Sscanf(first, "%d", &p)
	if p > 0 {
		return p, "", false
	}
	return 0, "", false
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

var _ = json.Marshal
var _ = fmt.Sprint
var _ = errors.New
