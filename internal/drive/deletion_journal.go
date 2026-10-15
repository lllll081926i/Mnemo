package drive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"mnemo-go/internal/model"
)

type DeletionJournal interface {
	ListDeletionRecords(string, string) ([]model.DeletionRecord, error)
	SaveDeletionRecords([]model.DeletionRecord) error
}

var deletionJournalState = struct {
	sync.RWMutex
	journal DeletionJournal
}{}

func SetDeletionJournal(journal DeletionJournal) {
	deletionJournalState.Lock()
	deletionJournalState.journal = journal
	deletionJournalState.Unlock()
}

func deletionJournal() DeletionJournal {
	deletionJournalState.RLock()
	defer deletionJournalState.RUnlock()
	return deletionJournalState.journal
}

func lockDeletionItems(ctx context.Context, account Context, fileIDs []string) (func(), error) {
	ordered := append([]string(nil), fileIDs...)
	sort.Strings(ordered)
	releases := []func(){}
	releaseAll := func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}
	previous := ""
	for _, fileID := range ordered {
		if fileID == "" || fileID == previous {
			continue
		}
		release, err := LockTarget(ctx, account.TokenFrom+":"+account.UserID, account.DriveID, "deletion-journal", fileID)
		if err != nil {
			releaseAll()
			return nil, err
		}
		releases = append(releases, release)
		previous = fileID
	}
	return releaseAll, nil
}

func journaledTrash(ctx context.Context, driver Driver, account Context, fileIDs []string) ([]string, error) {
	if !driver.Capabilities().LocalDeletionHistory {
		return driver.Trash(ctx, account, fileIDs)
	}
	journal := deletionJournal()
	if journal == nil {
		return nil, errors.New("删除记录存储未就绪，未执行远端删除")
	}
	release, err := lockDeletionItems(ctx, account, fileIDs)
	if err != nil {
		return nil, err
	}
	defer release()
	changes := make([]model.DeletionRecord, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		var file *model.File
		var err error
		if fresh, ok := driver.(FreshFileDriver); ok {
			file, err = fresh.GetFileFresh(ctx, account, fileID)
		} else {
			file, err = driver.GetFile(ctx, account, fileID)
		}
		if err != nil {
			return nil, fmt.Errorf("读取删除项元数据失败，未执行删除: %w", err)
		}
		if file == nil || file.FileID != fileID {
			return nil, errors.New("删除项元数据身份无效，未执行删除")
		}
		providerType := ""
		if account.Token != nil {
			providerType = account.Token.ProviderDriveType
		}
		changes = append(changes, model.DeletionRecord{UserID: account.UserID, DriveID: account.DriveID, FileID: fileID, Name: file.Name, ParentID: file.ParentFileID, Size: file.Size, IsDir: file.IsDir, ProviderType: providerType, IDKind: "driveItem", Status: "delete_pending", DeletedAt: time.Now().Unix()})
	}
	if err := journal.SaveDeletionRecords(changes); err != nil {
		return nil, fmt.Errorf("保存删除检查点失败，未执行删除: %w", err)
	}
	completed, remoteErr := driver.Trash(ctx, account, fileIDs)
	confirmed := map[string]bool{}
	for _, fileID := range completed {
		confirmed[fileID] = true
	}
	for index := range changes {
		changes[index].Status = "delete_unknown"
		if confirmed[changes[index].FileID] {
			changes[index].Status = "deleted"
		}
	}
	if err := journal.SaveDeletionRecords(changes); err != nil {
		return completed, errors.Join(remoteErr, fmt.Errorf("远端删除已执行，但本机结果保存失败，请核对官网回收站: %w", err))
	}
	return completed, remoteErr
}

func journaledRestore(ctx context.Context, driver Driver, account Context, fileIDs []string) ([]string, error) {
	if !driver.Capabilities().LocalDeletionHistory {
		return driver.Restore(ctx, account, fileIDs)
	}
	journal := deletionJournal()
	if journal == nil {
		return nil, errors.New("删除记录存储未就绪，未执行恢复")
	}
	release, err := lockDeletionItems(ctx, account, fileIDs)
	if err != nil {
		return nil, err
	}
	defer release()
	records, err := journal.ListDeletionRecords(account.UserID, account.DriveID)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, fileID := range fileIDs {
		selected[fileID] = true
	}
	changes := []model.DeletionRecord{}
	pendingIDs := []string{}
	confirmed := []string{}
	for _, record := range records {
		if !selected[record.FileID] {
			continue
		}
		if record.IDKind != "driveItem" {
			return nil, errors.New("回收站记录 ID 类型不匹配")
		}
		if record.Status == "restored" {
			confirmed = append(confirmed, record.FileID)
		} else if record.Status == "restore_pending" || record.Status == "restore_unknown" {
			file, err := driver.GetFile(ctx, account, record.FileID)
			if err != nil || file == nil || file.FileID != record.FileID {
				return confirmed, errors.New("上次恢复结果尚未确认，请先到官网核对；未重复提交恢复")
			}
			record.Status = "restored"
			confirmed = append(confirmed, record.FileID)
		} else {
			record.Status = "restore_pending"
			pendingIDs = append(pendingIDs, record.FileID)
		}
		changes = append(changes, record)
		delete(selected, record.FileID)
	}
	if err := journal.SaveDeletionRecords(changes); err != nil {
		return confirmed, fmt.Errorf("保存恢复检查点失败，未提交新恢复: %w", err)
	}
	if len(selected) != 0 {
		return confirmed, errors.New("项目没有当前账号的本机删除记录，请到官网恢复")
	}
	if len(pendingIDs) == 0 {
		return confirmed, nil
	}
	completed, remoteErr := driver.Restore(ctx, account, pendingIDs)
	completedSet := map[string]bool{}
	for _, fileID := range completed {
		completedSet[fileID] = true
	}
	for index := range changes {
		if changes[index].Status == "restored" {
			continue
		}
		changes[index].Status = "restore_unknown"
		if completedSet[changes[index].FileID] {
			changes[index].Status = "restored"
		}
	}
	if err := journal.SaveDeletionRecords(changes); err != nil {
		remoteErr = errors.Join(remoteErr, fmt.Errorf("远端恢复已执行，但本机结果保存失败，请核对目标目录: %w", err))
	}
	return append(confirmed, completed...), remoteErr
}
