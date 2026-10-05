package thunder

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) ListTrash(ctx context.Context, account drive.Context, _ *drive.ListOptions) ([]model.File, error) {
	return driver.List(ctx, account, RootID, &drive.ListOptions{Trashed: true})
}

func (*Driver) Restore(ctx context.Context, account drive.Context, ids []string) ([]string, error) {
	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	done := make([]string, 0, len(ids))
	err := withClient(ctx, account, func(cl *client) error {
		for _, id := range ids {
			if err := cl.mutate(ctx, http.MethodPatch, "/files/"+url.PathEscape(id)+"/untrash", map[string]any{}); err != nil {
				return err
			}
			done = append(done, id)
		}
		return nil
	})
	return done, err
}

func (*Driver) Delete(ctx context.Context, account drive.Context, refs []drive.FileRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	done := make([]string, 0, len(ids))
	err := withClient(ctx, account, func(cl *client) error {
		for _, id := range ids {
			entry, err := detail(ctx, cl, id)
			if err != nil {
				return err
			}
			if !entry.Trashed {
				return errors.New("迅雷永久删除仅支持回收站条目，请先将文件移入回收站")
			}
		}
		for _, id := range ids {
			if err := cl.mutate(ctx, http.MethodDelete, "/files/"+url.PathEscape(id)+"?space=", nil); err != nil {
				return err
			}
			done = append(done, id)
		}
		return nil
	})
	return done, err
}
