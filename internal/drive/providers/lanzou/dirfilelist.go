package lanzou

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

// fileList returns folders (task 47) followed by all file pages (task 5),
// mirroring apiLanzouFileList.
func (d *Driver) fileList(ctx context.Context, c drive.Context, folderID string) ([]LanzouItem, error) {
	fid := ToLanzouFolderId(folderID)
	foldersRes, err := d.doupload(ctx, c, url.Values{"task": {"47"}, "folder_id": {fid}})
	if err != nil {
		return nil, err
	}
	folders := decodeItems(foldersRes["text"])
	var files []LanzouItem
	for pg := 1; pg < 100; pg++ {
		filesRes, err := d.doupload(ctx, c, url.Values{"task": {"5"}, "folder_id": {fid}, "pg": {strconv.Itoa(pg)}})
		if err != nil {
			return nil, err
		}
		page := decodeItems(filesRes["text"])
		if len(page) == 0 {
			break
		}
		files = append(files, page...)
	}
	return append(folders, files...), nil
}

// decodeItems converts the raw "text" array of a doupload response.
func decodeItems(v any) []LanzouItem {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]LanzouItem, 0, len(arr))
	for _, r := range arr {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var it LanzouItem
		if json.Unmarshal(b, &it) == nil {
			out = append(out, it)
		}
	}
	return out
}

// mapLanzouItem converts a raw list entry into the unified file model
// (mirrors legacy mapLanzouItem).
func mapLanzouItem(item LanzouItem, driveID, parentID string) model.File {
	isDir := item.FolID != ""
	name := item.Name
	if !isDir && item.NameAll != "" {
		name = item.NameAll
	}
	id := item.ID
	if isDir {
		id = item.FolID
	}
	size := int64(0)
	if !isDir {
		size = parseSizeToBytes(item.Size)
	}
	parent := parentID
	if parentID == "-1" {
		parent = LANZOU_ROOT
	}
	timeUnix := int64(0)
	if item.Time == "" {
		// legacy sets time = Date.now() since the API has no parseable stamp
		timeUnix = time.Now().Unix()
	}
	f := driveutil.NewFile(driveID, id, parent, name, isDir, size, timeUnix)
	if item.Time != "" {
		f.TimeStr = item.Time
	}
	if isDir {
		f.Category = "folder"
		f.Icon = "iconfile-folder"
		f.SizeStr = ""
	}
	return f
}

// lowerExt returns the lowercased extension without the dot.
func lowerExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 || i == len(name)-1 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}
