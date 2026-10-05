package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func verifyMoveTarget(ctx context.Context, job *Job, source *model.File) error {
	item := job.Items[source.FileID]
	targetID := item.TargetID
	if source.IsDir {
		targetID = job.TargetDirectoryIDs[source.FileID]
	}
	if strings.TrimSpace(targetID) == "" {
		return errors.New("目标缺少文件 ID，无法确认复制结果，已保留来源")
	}
	if strings.TrimSpace(item.ParentID) == "" {
		return errors.New("目标缺少所属目录记录，无法可靠复查，已保留来源")
	}
	files, err := drive.ListDirAllContext(ctx, job.DstUser, job.DstDrive, item.ParentID, nil)
	if err != nil {
		return fmt.Errorf("复查迁移目标失败，已保留来源: %w", err)
	}
	var target *model.File
	for _, candidate := range files {
		if candidate.FileID == targetID || source.IsDir && strings.TrimRight(candidate.FileID, "/") == strings.TrimRight(targetID, "/") {
			target = &candidate
			break
		}
	}
	if err := compareMoveTarget(source, target); err != nil {
		return err
	}
	if !source.IsDir {
		item := job.Items[source.FileID]
		item.Verification = "remote-size"
		if source.ContentHash != "" && target.ContentHash != "" && source.ContentHashName != "" && strings.EqualFold(source.ContentHashName, target.ContentHashName) {
			item.Verification = "remote-hash"
		}
		job.Items[source.FileID] = item
	}
	return nil
}

func compareMoveTarget(source, target *model.File) error {
	if target == nil || target.IsDir != source.IsDir {
		return errors.New("迁移目标不存在或类型不符，已保留来源")
	}
	if source.IsDir {
		return nil
	}
	if source.Size < 0 || target.Size != source.Size {
		return errors.New("迁移目标大小不符或来源大小未知，已保留来源")
	}
	if source.ContentHash != "" && target.ContentHash != "" && source.ContentHashName != "" && strings.EqualFold(source.ContentHashName, target.ContentHashName) && !strings.EqualFold(source.ContentHash, target.ContentHash) {
		return errors.New("迁移目标内容哈希不符，已保留来源")
	}
	return nil
}
