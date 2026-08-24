package ilanzou

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

const listPageLimit = "30"

// fileList paginates /record/file/list (offset pages of 60) until totalPage is
// reached or a page comes back empty.
func (d *Driver) fileList(ctx context.Context, c drive.Context, folderID string) ([]listItem, error) {
	fid := ToILanzouFolderId(folderID)
	var all []listItem
	offset := 1
	for guard := 0; guard < 100; guard++ {
		j, _, err := d.request(ctx, c, "/record/file/list", requestOptions{
			method: http.MethodGet,
			query: map[string]string{
				"offset":   strconv.Itoa(offset),
				"limit":    listPageLimit,
				"folderId": fid,
				"type":     "0",
			},
		})
		if err != nil {
			return nil, err
		}
		list := rawItems(j["list"])
		if list == nil {
			if data := mapVal(j, "data"); data != nil {
				list = rawItems(data["list"])
			}
		}
		all = append(all, list...)
		totalPage := int(numOf(j["totalPage"]))
		if totalPage == 0 {
			if data := mapVal(j, "data"); data != nil {
				totalPage = int(numOf(data["totalPage"]))
			}
		}
		if totalPage <= 0 {
			totalPage = 1
		}
		if offset >= totalPage || len(list) == 0 {
			break
		}
		offset++
	}
	return all, nil
}

func rawItems(v any) []listItem {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]listItem, 0, len(arr))
	for _, r := range arr {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var it listItem
		if json.Unmarshal(b, &it) == nil {
			out = append(out, it)
		}
	}
	return out
}

// mapILanzouItem converts a raw list entry into the unified file model
// (mirrors legacy mapILanzouItem; fileSize is in KB).
func mapILanzouItem(item listItem, driveID, parentID string) model.File {
	isDir := item.FileType == 2
	name := item.FileName
	if isDir {
		name = item.FolderName
	}
	id := ""
	if isDir {
		id = strconv.FormatInt(item.FolderID, 10)
	} else {
		id = strconv.FormatInt(item.FileID, 10)
	}
	size := int64(0)
	if !isDir {
		size = item.FileSize * 1024
	}
	parent := parentID
	if parentID == "0" {
		parent = ILANZOU_ROOT
	}
	timeUnix := int64(0)
	if item.UpdTime != "" {
		if t, err := parseUpdTime(item.UpdTime); err == nil {
			timeUnix = t.Unix()
		}
	}
	if timeUnix == 0 {
		timeUnix = time.Now().Unix()
	}
	f := driveutil.NewFile(driveID, id, parent, name, isDir, size, timeUnix)
	if isDir {
		f.Category = "folder"
		f.Icon = "iconfile-folder"
		f.SizeStr = ""
	}
	return f
}

var updTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02 15:04",
	"2006/01/02",
	"2006-01-02T15:04:05",
	time.RFC3339,
}

// parseUpdTime parses the server updTime string in local time (JS Date.parse
// semantics: slashes/date-only strings are treated as local wall clock).
func parseUpdTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range updTimeLayouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("ilanzou: 无法解析时间 " + value)
}
