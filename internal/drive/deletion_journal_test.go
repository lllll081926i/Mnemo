package drive

import (
	"context"
	"errors"
	"testing"

	"mnemo-go/internal/model"
)

type journalTestStore struct {
	records []model.DeletionRecord
	saves   int
	failAt  int
}

func (storage *journalTestStore) ListDeletionRecords(string, string) ([]model.DeletionRecord, error) {
	return append([]model.DeletionRecord(nil), storage.records...), nil
}
func (storage *journalTestStore) SaveDeletionRecords(records []model.DeletionRecord) error {
	storage.saves++
	if storage.saves == storage.failAt {
		return errors.New("模拟存储故障")
	}
	storage.records = append([]model.DeletionRecord(nil), records...)
	return nil
}

type journalTestDriver struct {
	Driver
	trashes, restores int
	live              bool
}

func (*journalTestDriver) List(context.Context, Context, string, *ListOptions) ([]model.File, error) {
	return nil, nil
}
func (*journalTestDriver) Capabilities() Capabilities {
	return Capabilities{LocalDeletionHistory: true}
}
func (driver *journalTestDriver) GetFile(_ context.Context, _ Context, fileID string) (*model.File, error) {
	if !driver.live {
		return nil, errors.New("项目不可见")
	}
	return &model.File{FileID: fileID, Name: "原文件"}, nil
}
func (driver *journalTestDriver) Trash(_ context.Context, _ Context, fileIDs []string) ([]string, error) {
	driver.trashes++
	driver.live = false
	return fileIDs, nil
}
func (driver *journalTestDriver) Restore(_ context.Context, _ Context, fileIDs []string) ([]string, error) {
	driver.restores++
	driver.live = true
	return fileIDs, nil
}

func TestDeletionCheckpointPrecedesRemoteDelete(testContext *testing.T) {
	previous := deletionJournal()
	testContext.Cleanup(func() { SetDeletionJournal(previous) })
	for _, failAt := range []int{1, 2} {
		storage := &journalTestStore{failAt: failAt}
		SetDeletionJournal(storage)
		driver := &journalTestDriver{live: true}
		completed, err := journaledTrash(testContext.Context(), driver, Context{UserID: "user", DriveID: "drive"}, []string{"file"})
		if err == nil || driver.trashes != failAt-1 {
			testContext.Fatalf("检查点失败时删除次数错误: %+v %v", driver, err)
		}
		if failAt == 2 && (len(completed) != 1 || len(storage.records) != 1 || storage.records[0].Status != "delete_pending") {
			testContext.Fatalf("丢失待确认记录: %+v", storage)
		}
	}
}

func TestRestoreLostLocalReceiptDoesNotRepeatPost(testContext *testing.T) {
	previous := deletionJournal()
	testContext.Cleanup(func() { SetDeletionJournal(previous) })
	storage := &journalTestStore{failAt: 2, records: []model.DeletionRecord{{UserID: "user", DriveID: "drive", FileID: "file", IDKind: "driveItem", Status: "deleted"}}}
	SetDeletionJournal(storage)
	driver := &journalTestDriver{}
	account := Context{UserID: "user", DriveID: "drive"}
	if _, err := journaledRestore(testContext.Context(), driver, account, []string{"file"}); err == nil {
		testContext.Fatal("未报告本机回执保存失败")
	}
	for repeat := 0; repeat < 2; repeat++ {
		completed, err := journaledRestore(testContext.Context(), driver, account, []string{"file"})
		if err != nil || len(completed) != 1 || driver.restores != 1 || storage.records[0].Status != "restored" {
			testContext.Fatalf("重复恢复或未对账: %+v %v", storage, err)
		}
	}
}

func TestUnknownRestoreAndForeignIDDoNotWrite(testContext *testing.T) {
	previous := deletionJournal()
	testContext.Cleanup(func() { SetDeletionJournal(previous) })
	storage := &journalTestStore{records: []model.DeletionRecord{{FileID: "file", IDKind: "driveItem", Status: "restore_unknown"}}}
	SetDeletionJournal(storage)
	driver := &journalTestDriver{}
	for _, fileID := range []string{"file", "foreign"} {
		if _, err := journaledRestore(testContext.Context(), driver, Context{}, []string{fileID}); err == nil || driver.restores != 0 {
			testContext.Fatalf("未知或越界项目发起恢复: %v", err)
		}
	}
}
