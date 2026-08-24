package pan123

import (
	"context"
	"net/http"

	"mnemo-go/internal/drive"
)

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	res := &drive.MkdirResult{}
	resp, err := d.api(ctx, c, http.MethodPost, apiUploadReq, map[string]any{
		"driveId":      0,
		"etag":         "",
		"fileName":     name,
		"parentFileId": toPan123Number(parentID),
		"size":         0,
		"type":         1,
	}, nil)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	data := parseMap(resp.Data)
	res.FileID = firstString(data, "FileId", "fileId")
	return res, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	_, err := d.api(ctx, c, http.MethodPost, apiRename, map[string]any{
		"driveId":  0,
		"fileId":   toPan123Number(fileID),
		"fileName": name,
	}, nil)
	if err != nil {
		return nil, err
	}
	result := &drive.RenameResult{FileID: fileID, Name: name}
	if detail, err := d.GetFile(ctx, c, fileID); err == nil {
		result.ParentFileID = detail.ParentFileID
		result.IsDir = detail.IsDir
	}
	return result, nil
}

// batchOp applies fn to each id, collecting the successful ones (legacy skips
// per-item failures).
func (d *Driver) batchOp(ctx context.Context, c drive.Context, ids []string, fn func(ctx context.Context, c drive.Context, id int64) error) ([]string, error) {
	var ok []string
	for _, id := range ids {
		if err := fn(ctx, c, toPan123Number(id)); err != nil {
			continue
		}
		ok = append(ok, id)
	}
	return ok, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return d.batchOp(ctx, c, fileIDs, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiTrash, map[string]any{
			"driveId":           0,
			"operation":         true,
			"fileTrashInfoList": []any{map[string]any{"FileId": id}},
		}, nil)
		return err
	})
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return d.batchOp(ctx, c, fileIDs, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiTrash, map[string]any{
			"driveId":           0,
			"operation":         false,
			"fileTrashInfoList": []any{map[string]any{"FileId": id}},
		}, nil)
		return err
	})
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	return d.batchOp(ctx, c, ids, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiDelete, map[string]any{
			"driveId":           0,
			"operation":         true,
			"fileTrashInfoList": []any{map[string]any{"FileId": id}},
		}, nil)
		return err
	})
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	parent := toPan123Number(toParentID)
	return d.batchOp(ctx, c, ids, func(ctx context.Context, c drive.Context, id int64) error {
		_, err := d.api(ctx, c, http.MethodPost, apiMove, map[string]any{
			"fileIdList":   []any{map[string]any{"FileId": id}},
			"parentFileId": parent,
		}, nil)
		return err
	})
}

// Copy: AList 123 web 不支持服务端复制（legacy copy 直接返回空数组）。
func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	return []string{}, nil
}
