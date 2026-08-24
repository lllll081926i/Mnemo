package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer/migrate"
)

// MigrateFiles copies/moves files across two accounts.
func (a *App) MigrateFiles(srcUser, srcDrive, dstUser, dstDrive, dstParent string, fileIDs []string, move bool) (job *migrate.Job, retErr error) {
	started := logActionStarted("创建迁移任务", "pan", srcUser, srcDrive,
		"file_count", len(fileIDs), "move", move,
		"target_provider", drive.ProviderOf(dstUser, dstDrive, ""))
	defer func() {
		fields := []any{"file_count", len(fileIDs), "move", move,
			"target_provider", drive.ProviderOf(dstUser, dstDrive, "")}
		if job != nil {
			fields = append(fields, "job_id", redactID(job.ID))
		}
		logActionFinished("创建迁移任务", "pan", srcUser, srcDrive, started, retErr, fields...)
	}()
	if strings.TrimSpace(srcUser) == "" || strings.TrimSpace(srcDrive) == "" ||
		strings.TrimSpace(dstUser) == "" || strings.TrimSpace(dstDrive) == "" {
		return nil, fmt.Errorf("迁移账号信息不完整")
	}
	if len(fileIDs) == 0 {
		return nil, fmt.Errorf("请选择要迁移的文件")
	}
	if err := migrate.ValidateEndpoints(srcUser, srcDrive, dstUser, dstDrive); err != nil {
		return nil, err
	}
	srcCaps := drive.RegistryCaps(drive.ProviderOf(srcUser, srcDrive, ""))
	dstCaps := drive.RegistryCaps(drive.ProviderOf(dstUser, dstDrive, ""))
	if !srcCaps.Download {
		return nil, fmt.Errorf("源网盘不支持下载，无法迁移")
	}
	if !dstCaps.Upload && len(dstCaps.RapidUploadHashes) == 0 {
		return nil, fmt.Errorf("目标网盘不支持上传，无法迁移")
	}
	if dstParent == "" {
		dstParent = "root"
	}
	ids := append([]string(nil), fileIDs...)
	job = &migrate.Job{
		ID:      "mig-" + fmt.Sprint(time.Now().UnixNano()),
		SrcUser: srcUser, SrcDrive: srcDrive, FileIDs: ids,
		DstUser: dstUser, DstDrive: dstDrive, DstParent: dstParent, Move: move,
		Status: "pending",
	}
	eng := a.migrationEngine()
	if eng == nil {
		return nil, fmt.Errorf("迁移服务未启动")
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveMigrateJob(job); err != nil {
		return nil, err
	}
	a.emit("migrate:progress", *job)
	// derive from the app context so migrations are canceled on shutdown
	ctx := a.appContext()
	go a.runMigrateJob(ctx, eng, job, "new")
	return job, nil
}

func (a *App) runMigrateJob(ctx context.Context, eng *migrate.Engine, job *migrate.Job, source string) {
	if eng == nil || job == nil {
		return
	}
	started := logActionStarted("执行迁移任务", "transfer", job.SrcUser, job.SrcDrive,
		"job_id", redactID(job.ID), "file_count", len(job.FileIDs), "move", job.Move,
		"source", source, "target_provider", drive.ProviderOf(job.DstUser, job.DstDrive, ""))
	err := eng.Run(ctx, job)
	logActionFinished("执行迁移任务", "transfer", job.SrcUser, job.SrcDrive, started, err,
		"job_id", redactID(job.ID), "status", job.Status,
		"processed_count", job.Processed, "failed_count", job.Failed)
}

// ListMigrateJobs returns persisted migration jobs, including jobs from a
// previous process which were recovered as canceled during startup.
func (a *App) ListMigrateJobs() []migrate.Job {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListMigrateJobs()
	if err != nil {
		return nil
	}
	return list
}

// CancelMigrate cancels an active migration without deleting its history.
func (a *App) CancelMigrate(id string) {
	started := logActionStarted("取消迁移任务", "transfer", "", "", "job_id", redactID(id))
	if eng := a.migrationEngine(); eng != nil {
		eng.Cancel(id)
		logActionFinished("取消迁移任务", "transfer", "", "", started, nil, "job_id", redactID(id))
		return
	}
	logActionFinished("取消迁移任务", "transfer", "", "", started, errors.New("迁移服务未启动"), "job_id", redactID(id))
}

// ResumeMigrate retries only the resources that did not reach a persisted
// completion checkpoint. It deliberately requires an explicit user action:
// jobs interrupted by application shutdown are recovered as canceled rather
// than silently generating network traffic at the next startup.
func (a *App) ResumeMigrate(id string) (job *migrate.Job, retErr error) {
	id = strings.TrimSpace(id)
	started := logActionStarted("恢复迁移任务", "transfer", "", "", "job_id", redactID(id))
	defer func() {
		fields := []any{"job_id", redactID(id)}
		if job != nil {
			fields = append(fields, "file_count", len(job.FileIDs), "status", job.Status)
		}
		logActionFinished("恢复迁移任务", "transfer", "", "", started, retErr, fields...)
	}()
	if id == "" {
		return nil, fmt.Errorf("迁移任务 ID 不能为空")
	}
	eng := a.migrationEngine()
	if eng == nil {
		return nil, fmt.Errorf("迁移服务未启动")
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	jobs, err := st.ListMigrateJobs()
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		if jobs[i].ID != id {
			continue
		}
		candidate := jobs[i]
		switch candidate.Status {
		case "canceled", "partial", "failed":
			// These terminal states can safely be retried from their durable
			// per-resource checkpoints.
		case "pending", "running":
			return nil, fmt.Errorf("迁移任务正在运行")
		default:
			return nil, fmt.Errorf("迁移任务状态 %q 不能恢复", candidate.Status)
		}
		if len(candidate.FileIDs) == 0 {
			return nil, fmt.Errorf("迁移任务没有可恢复的文件")
		}
		candidate.Status = "pending"
		candidate.Message = "准备恢复未完成资源"
		if err := st.SaveMigrateJob(&candidate); err != nil {
			return nil, err
		}
		job = &candidate
		a.emit("migrate:progress", *job)
		// Derive from the app context so a resumed job is canceled on shutdown.
		ctx := a.appContext()
		go a.runMigrateJob(ctx, eng, job, "resume")
		return job, nil
	}
	return nil, fmt.Errorf("迁移任务不存在")
}

// DeleteMigrateJob removes one migration history record.
func (a *App) DeleteMigrateJob(id string) (retErr error) {
	started := logActionStarted("删除迁移记录", "transfer", "", "", "job_id", redactID(id))
	defer func() {
		logActionFinished("删除迁移记录", "transfer", "", "", started, retErr, "job_id", redactID(id))
	}()
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	retErr = st.DeleteMigrateJob(id)
	return retErr
}

// ClearMigrateJobs removes all finished migration history records.
func (a *App) ClearMigrateJobs() (retErr error) {
	started := logActionStarted("清理迁移记录", "transfer", "", "")
	defer func() { logActionFinished("清理迁移记录", "transfer", "", "", started, retErr) }()
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	retErr = st.ClearMigrateJobs()
	return retErr
}

// uploadSessionAdapter bridges store.UploadSession* into the
// drive.UploadSessionStore interface.
type uploadSessionAdapter struct{}

func (uploadSessionAdapter) SaveUploadSession(key string, partNumbers []int) error {
	return store.SaveUploadSession(key, partNumbers)
}

func (uploadSessionAdapter) SaveUploadSessionState(key, sessionID string, partNumbers []int) error {
	return store.SaveUploadSessionState(key, sessionID, partNumbers)
}
func (uploadSessionAdapter) LoadUploadSession(key string) []int {
	return store.LoadUploadSession(key)
}

func (uploadSessionAdapter) LoadUploadSessionState(key string) (string, []int) {
	return store.LoadUploadSessionState(key)
}
func (uploadSessionAdapter) ClearUploadSession(key string) {
	store.ClearUploadSession(key)
}
