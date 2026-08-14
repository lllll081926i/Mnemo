package lanzou

import (
	"context"
	"net/url"

	"mnemo-go/internal/drive"
)

// apiLanzouMkdir ports /task=2.
func (d *Driver) mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	res, err := d.doupload(ctx, c, url.Values{
		"task":               {"2"},
		"parent_id":          {ToLanzouFolderId(parentID)},
		"folder_name":        {name},
		"folder_description": {""},
	})
	if err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	id := strOf(res["text"])
	if id == "" {
		id = strOf(res["info"])
	}
	if id == "" {
		return &drive.MkdirResult{Error: "新建文件夹失败"}, nil
	}
	return &drive.MkdirResult{FileID: id}, nil
}

// apiLanzouRenameFile ports /task=46 (files only).
func (d *Driver) renameFile(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	_, err := d.doupload(ctx, c, url.Values{
		"task":      {"46"},
		"file_id":   {fileID},
		"file_name": {name},
		"type":      {"2"},
	})
	if err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name}, nil
}

// apiLanzouMoveFile ports /task=20 (files only).
func (d *Driver) moveFile(ctx context.Context, c drive.Context, fileID, folderID string) error {
	_, err := d.doupload(ctx, c, url.Values{
		"task":      {"20"},
		"folder_id": {ToLanzouFolderId(folderID)},
		"file_id":   {fileID},
	})
	return err
}

// apiLanzouRemove ports /task=6 (file) or /task=3 (folder).
func (d *Driver) removeItem(ctx context.Context, c drive.Context, id string, isDir bool) error {
	if isDir {
		_, err := d.doupload(ctx, c, url.Values{"task": {"3"}, "folder_id": {id}})
		return err
	}
	_, err := d.doupload(ctx, c, url.Values{"task": {"6"}, "file_id": {id}})
	return err
}

// apiLanzouFileShare ports /task=22 (file) or /task=18 (folder).
func (d *Driver) fileShare(ctx context.Context, c drive.Context, fileID string, isDir bool) (map[string]any, error) {
	task := "22"
	if isDir {
		task = "18"
	}
	j, err := d.doupload(ctx, c, url.Values{"task": {task}, "file_id": {fileID}})
	if err != nil {
		return nil, err
	}
	if info := mapVal(j, "info"); info != nil {
		return info, nil
	}
	if text := mapVal(j, "text"); text != nil {
		return text, nil
	}
	return map[string]any{}, nil
}
