package pan189

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const recyclePageSize = 100

func (d *Driver) ListTrash(ctx context.Context, c drive.Context, _ *drive.ListOptions) ([]model.File, error) {
	session, err := sessionOf(c.Token)
	if err != nil {
		return nil, err
	}
	spaces := []string{spacePersonal}
	if session.FamilyID != "" && session.FamilySessionKey != "" && session.FamilySessionSecret != "" {
		spaces = append(spaces, spaceFamily)
	}
	items := make([]model.File, 0)
	seen := make(map[string]bool)
	for _, space := range spaces {
		spaceStart := len(items)
		for page := 1; page <= 200; page++ {
			family := space == spaceFamily
			query := map[string]string{
				"pageNum": strconv.Itoa(page), "pageSize": strconv.Itoa(recyclePageSize),
				"iconOption": "1", "family": strconv.FormatBool(family),
			}
			if family {
				query["familyId"] = session.FamilyID
			}
			raw, err := d.request(ctx, c, apiURL+"/listRecycleBinFiles.action", reqOptions{
				method: "GET", query: query, family: boolPtr(family),
			})
			if err != nil {
				return nil, err
			}
			var response struct {
				Count      int               `json:"count"`
				FileList   []json.RawMessage `json:"fileList"`
				FolderList []json.RawMessage `json:"folderList"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				return nil, fmt.Errorf("天翼回收站列表响应无效: %w", err)
			}
			for _, group := range []struct {
				entries []json.RawMessage
				isDir   bool
			}{{response.FolderList, true}, {response.FileList, false}} {
				for _, entry := range group.entries {
					var value struct {
						ID         listEntryID `json:"id"`
						Name       string      `json:"name"`
						Size       int64       `json:"size"`
						MD5        string      `json:"md5"`
						LastOpTime string      `json:"lastOpTime"`
						CreateDate string      `json:"createDate"`
						Path       string      `json:"pathStr"`
					}
					if err := json.Unmarshal(entry, &value); err != nil {
						return nil, fmt.Errorf("天翼回收站条目无效: %w", err)
					}
					if strings.TrimSpace(string(value.ID)) == "" || strings.TrimSpace(value.Name) == "" {
						return nil, errors.New("天翼回收站包含无效文件")
					}
					file := mapFile(pan189File{
						ID: string(value.ID), Name: value.Name, Size: value.Size, MD5: value.MD5,
						LastOpTime: value.LastOpTime, CreateDate: value.CreateDate, IsFolder: group.isDir,
					}, c.DriveID, space, Pan189DefaultFolder)
					if seen[file.FileID] {
						return nil, errors.New("天翼回收站包含重复的文件 ID")
					}
					seen[file.FileID] = true
					file.Path = value.Path
					items = append(items, file)
				}
			}
			pageCount := len(response.FileList) + len(response.FolderList)
			if pageCount < recyclePageSize || response.Count > 0 && len(items)-spaceStart >= response.Count {
				break
			}
			if page == 200 {
				return nil, errors.New("天翼回收站分页超过上限")
			}
		}
	}
	return items, nil
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}
	files, err := d.ListTrash(ctx, c, nil)
	if err != nil {
		return nil, err
	}
	available := make(map[string]model.File, len(files))
	for _, file := range files {
		available[file.FileID] = file
	}
	groups := map[string][]fileRefItem{}
	var failed []error
	for _, id := range fileIDs {
		file, ok := available[id]
		if !ok {
			failed = append(failed, fmt.Errorf("%s: 回收站中未找到文件", id))
			continue
		}
		space, rawID := pan189SpaceID(id)
		item := fileRefItem{FileID: rawID, FileName: file.Name}
		if file.IsDir {
			item.IsFolder = 1
		}
		groups[space] = append(groups[space], item)
	}
	var restored []string
	for _, space := range []string{spacePersonal, spaceFamily} {
		if len(groups[space]) == 0 {
			continue
		}
		ids, err := d.runBatch(ctx, c, space, "RESTORE", groups[space], "", 250*time.Millisecond, nil)
		if err != nil {
			failed = append(failed, err)
		} else {
			restored = append(restored, ids...)
		}
	}
	return restored, errors.Join(failed...)
}
