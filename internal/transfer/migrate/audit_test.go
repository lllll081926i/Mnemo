package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestMoveTargetComparison(t *testing.T) {
	source := &model.File{Size: 8, ContentHashName: "sha1", ContentHash: "ABC"}
	for _, testCase := range []struct {
		name   string
		target *model.File
		valid  bool
	}{
		{"缺失", nil, false},
		{"类型不符", &model.File{IsDir: true, Size: 8}, false},
		{"大小不符", &model.File{Size: 7}, false},
		{"哈希不符", &model.File{Size: 8, ContentHashName: "SHA1", ContentHash: "DEF"}, false},
		{"哈希大小写", &model.File{Size: 8, ContentHashName: "SHA1", ContentHash: "abc"}, true},
		{"不同哈希算法只验证大小", &model.File{Size: 8, ContentHashName: "md5", ContentHash: "DEF"}, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := compareMoveTarget(source, testCase.target); (err == nil) != testCase.valid {
				t.Fatalf("目标验证错误: %v", err)
			}
		})
	}
}

func TestMoveChecksDestinationBeforeDeletingSource(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		targetID    string
		targetSize  int64
		wantDeleted bool
	}{
		{"缺少回执", "", 8, false},
		{"目标消失", "/missing", 8, false},
		{"目标截断", "/target", 7, false},
		{"目标完整", "/target", 8, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deleted := false
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodDelete {
					deleted = true
					response.WriteHeader(http.StatusNoContent)
					return
				}
				if request.Method != "PROPFIND" || request.URL.Path == "/missing" {
					response.WriteHeader(http.StatusNotFound)
					return
				}
				size := int64(8)
				responsePath := request.URL.Path
				if responsePath == "/" {
					responsePath = "/source"
				}
				if request.URL.Path == "/target" {
					size = testCase.targetSize
				}
				response.WriteHeader(http.StatusMultiStatus)
				entry := fmt.Sprintf(`<d:response><d:href>%s</d:href><d:propstat><d:prop><d:resourcetype/><d:getcontentlength>%d</d:getcontentlength><d:getlastmodified>Mon, 05 Oct 2026 00:00:00 GMT</d:getlastmodified></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, responsePath, size)
				if request.URL.Path == "/" {
					entry += fmt.Sprintf(`<d:response><d:href>/target</d:href><d:propstat><d:prop><d:resourcetype/><d:getcontentlength>%d</d:getcontentlength></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, testCase.targetSize)
				}
				_, _ = fmt.Fprintf(response, `<d:multistatus xmlns:d="DAV:">%s</d:multistatus>`, entry)
			}))
			defer server.Close()
			drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
				return &model.TokenInfo{Conn: &model.ConnConfig{Endpoint: server.URL}}, nil
			})
			defer drive.SetTokenResolver(nil)
			job := &Job{Move: true, SrcUser: "webdav_source_" + testCase.name, SrcDrive: "webdav:source", DstUser: "webdav_target_" + testCase.name, DstDrive: "webdav:target", Items: map[string]model.MigrateItem{"/source": {TargetID: testCase.targetID, ParentID: "/"}}}
			source, err := drive.GetFileFreshContext(t.Context(), job.SrcUser, job.SrcDrive, "/source")
			if err != nil {
				t.Fatal(err)
			}
			err = NewEngine(nil, nil).finalizeMove(t.Context(), job, source)
			if deleted != testCase.wantDeleted || (err == nil) != testCase.wantDeleted {
				t.Fatalf("目标未经验证不得删源: deleted=%v, err=%v", deleted, err)
			}
			if !testCase.wantDeleted {
				job.ID = "resume-" + testCase.name
				job.FileIDs, job.CopiedFileIDs = []string{"/source"}, []string{"/source"}
				item := job.Items["/source"]
				item.SourceRecorded, item.SourceTime, item.SourceHash, item.Size = true, source.Time, sourceHash(source), source.Size
				job.Items["/source"] = item
				if err := NewEngine(nil, nil).Run(t.Context(), job); err == nil || deleted || job.Status != "partial" {
					t.Fatalf("旧复制检查点恢复时绕过了目标验证: deleted=%v, status=%s, err=%v", deleted, job.Status, err)
				}
			}
		})
	}
}

func TestCancelAllWaitsForMigrationCleanupAndRejectsNewRun(t *testing.T) {
	engine := NewEngine(nil, nil)
	ctx, cancel, registered := engine.registerCancel(t.Context(), "active")
	if !registered {
		t.Fatal("注册失败")
	}
	defer cancel()
	closed := make(chan struct{})
	go func() { engine.CancelAll(); close(closed) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("未取消正在运行的迁移")
	}
	select {
	case <-closed:
		t.Fatal("退出未等待迁移保存状态和清理临时文件")
	default:
	}
	engine.releaseCancel("active")
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("收尾完成后仍未退出")
	}
	if err := engine.Run(t.Context(), &Job{ID: "late"}); err == nil {
		t.Fatal("已关闭的引擎不应接受新迁移")
	}
}

func TestCheckpointFailuresArePropagated(t *testing.T) {
	directory := t.TempDir()
	storage, err := store.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "migrate_jobs.json"), 0700); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(storage, nil)
	job := &Job{ID: "checkpoint-failure"}
	for _, operation := range []func() error{
		func() error { return engine.markTargetDirectory(job, "directory", "target") },
		func() error { return engine.markCompleted(job, "completed") },
		func() error { return engine.markCopied(job, "copied") },
	} {
		if err := operation(); !errors.Is(err, errCheckpoint) {
			t.Fatalf("保存失败被忽略: %v", err)
		}
	}
	if jobHasID(job.CopiedFileIDs, "copied") {
		t.Fatal("保存失败的复制记录不应允许重试时删源")
	}
	if err := engine.Run(t.Context(), &Job{ID: "run-failure"}); !errors.Is(err, errCheckpoint) {
		t.Fatalf("Run 丢失保存错误: %v", err)
	}
}

func TestPartialErrorsPreserveCancellationAndCheckpointCause(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errCheckpoint} {
		wrapped := newPartialMigrationError(cause, 1)
		if !isPartialError(wrapped) || !errors.Is(wrapped, cause) {
			t.Fatalf("丢失原始原因: %v", wrapped)
		}
	}
}

func TestRunRejectsDuplicateAndEmptyFileIDs(t *testing.T) {
	for _, ids := range [][]string{{"file", "file"}, {" "}} {
		job := &Job{ID: "invalid", FileIDs: ids}
		if err := NewEngine(nil, nil).Run(t.Context(), job); err == nil || job.Status != "" {
			t.Fatalf("非法选择被执行: %+v", job)
		}
	}
}

func TestProgressSnapshotDoesNotChangeWithWorkerState(t *testing.T) {
	var snapshot *Job
	engine := NewEngine(nil, func(job *Job) { snapshot = job })
	job := &Job{ID: "snapshot", FileIDs: []string{"file"}, CompletedFileIDs: []string{"completed"}, CopiedFileIDs: []string{"copied"}, TargetDirectoryIDs: map[string]string{"directory": "original"}, Items: map[string]model.MigrateItem{"file": {Status: "running"}}}
	engine.emit(job)
	job.FileIDs[0] = "changed"
	job.CompletedFileIDs[0] = "changed"
	job.CopiedFileIDs[0] = "changed"
	job.TargetDirectoryIDs["directory"] = "changed"
	job.Items["file"] = model.MigrateItem{Status: "completed"}
	if snapshot.FileIDs[0] != "file" || snapshot.CompletedFileIDs[0] != "completed" || snapshot.CopiedFileIDs[0] != "copied" || snapshot.TargetDirectoryIDs["directory"] != "original" || snapshot.Items["file"].Status != "running" {
		t.Fatalf("发给界面的数据被后台改写: %+v", snapshot)
	}
}

func TestCopyToWebDAVRecordsActualRenamedTargetID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			_, _ = io.WriteString(response, "data")
		case "PROPFIND":
			if request.URL.Path == "/source" {
				response.WriteHeader(http.StatusMultiStatus)
				_, _ = io.WriteString(response, `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/source</d:href><d:propstat><d:prop><d:resourcetype/><d:getcontentlength>4</d:getcontentlength></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
			} else {
				response.WriteHeader(http.StatusNotFound)
			}
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, request.Body)
			response.WriteHeader(http.StatusCreated)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		return &model.TokenInfo{Conn: &model.ConnConfig{Endpoint: server.URL}}, nil
	})
	defer drive.SetTokenResolver(nil)
	job := &Job{SrcUser: "webdav_source_id", SrcDrive: "webdav:source", DstUser: "webdav_target_id", DstDrive: "webdav:target", Items: map[string]model.MigrateItem{"/source": {}}}
	if err := NewEngine(nil, nil).spoolMigrate(t.Context(), job, &model.File{FileID: "/source", Name: "file.txt", Size: 4}, "/target"); err != nil {
		t.Fatal(err)
	}
	if job.Items["/source"].TargetID != "/target/file.txt" {
		t.Fatalf("未记录真实目标: %+v", job.Items)
	}
}
