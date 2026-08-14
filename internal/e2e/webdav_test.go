// Package e2e contains end-to-end integration tests exercising the real drive
// pipeline (store → ops facade → provider driver → HTTP client) against a
// local in-process WebDAV server. No external services are needed.
package e2e

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/webdav"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers" // register all providers
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// startWebDAV spins up an in-process WebDAV server over a temp dir.
func startWebDAV(t *testing.T, dir string) (url string, stop func()) {
	t.Helper()
	handler := &webdav.Handler{
		FileSystem: webdav.Dir(dir),
		LockSystem: webdav.NewMemLS(),
	}
	srv := &http.Server{Handler: handler}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	return "http://" + ln.Addr().String(), func() { _ = srv.Close() }
}

// TestWebDAVEndToEnd runs list/mkdir/upload/download/rename/move/copy/delete
// through the real drive facade against a live local WebDAV server.
func TestWebDAVEndToEnd(t *testing.T) {
	dir := t.TempDir()
	url, stop := startWebDAV(t, dir)
	defer stop()

	// 1. persist a mounted webdav account
	st, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	uid := model.BuildUserID("webdav", "e2e")
	tok := &model.TokenInfo{
		TokenFrom: model.ProviderWebdav,
		UserID:    uid,
		UserName:  "e2e",
		Conn:      &model.ConnConfig{Endpoint: url},
	}
	driveID := model.BuildDriveID("webdav", "e2e")
	acc := &model.Account{UserID: uid, DriveID: driveID, Token: tok}
	if err := st.SaveAccount(acc); err != nil {
		t.Fatal(err)
	}
	drive.SetTokenResolver(func(userID, _ string) (*model.TokenInfo, error) {
		a, err := st.GetAccount(userID)
		if err != nil {
			return nil, err
		}
		return a.Token, nil
	})

	// 2. list root (empty)
	files, err := drive.ListDir(uid, driveID, "/", nil)
	if err != nil {
		t.Fatalf("list root: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("root should be empty, got %d", len(files))
	}

	// 3. mkdir
	mk, err := drive.Mkdir(uid, driveID, "/", "sub")
	if err != nil || mk == nil || mk.FileID == "" {
		t.Fatalf("mkdir: %v %+v", err, mk)
	}

	// 4. upload a file into sub
	payload := []byte("hello mnemo e2e")
	upPath := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(upPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	ui := &model.UploadingUI{
		UploadID: "up-1",
		Info: model.UploadInfo{
			LocalFilePath: upPath, ParentFileID: mk.FileID,
			DriveID: driveID, Name: "hello.txt", Size: int64(len(payload)),
		},
		Upload: model.UploadState{},
	}
	handler, err := drive.QueueUploadHandler(uid, driveID)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler(context.Background(), ui); err != nil {
		t.Fatalf("upload: %v", err)
	}

	// 5. list sub → hello.txt present
	files, err = drive.ListDir(uid, driveID, mk.FileID, nil)
	if err != nil {
		t.Fatalf("list sub: %v", err)
	}
	if len(files) != 1 || files[0].Name != "hello.txt" {
		t.Fatalf("expected hello.txt, got %+v", files)
	}
	if files[0].Size != int64(len(payload)) {
		t.Fatalf("size mismatch: %d", files[0].Size)
	}

	// 6. download URL + fetch content
	dl, err := drive.GetDownloadURL(uid, driveID, files[0].FileID, 60)
	if err != nil {
		t.Fatalf("download url: %v", err)
	}
	resp, err := http.Get(dl.URL)
	if err != nil {
		t.Fatalf("get url: %v", err)
	}
	buf := make([]byte, len(payload))
	n, _ := resp.Body.Read(buf)
	resp.Body.Close()
	if string(buf[:n]) != string(payload) {
		t.Fatalf("download content mismatch: %q", buf[:n])
	}

	// 7. rename
	if _, err := drive.RenameBatch(uid, driveID, []drive.FileRef{{ID: files[0].FileID}}, []string{"renamed.txt"}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	files, _ = drive.ListDir(uid, driveID, mk.FileID, nil)
	if len(files) != 1 || files[0].Name != "renamed.txt" {
		t.Fatalf("rename failed: %+v", files)
	}

	// 8. move to root
	rid, _ := drive.RootID(uid, driveID)
	if _, err := drive.MoveBatch(uid, driveID, []drive.FileRef{{ID: files[0].FileID}}, rid, ""); err != nil {
		t.Fatalf("move: %v", err)
	}
	rootFiles, _ := drive.ListDir(uid, driveID, "/", nil)
	if !hasFile(rootFiles, "renamed.txt") {
		t.Fatalf("move failed, renamed.txt missing from root: %+v", names(rootFiles))
	}

	// 9. copy
	if _, err := drive.CopyBatch(uid, driveID, []drive.FileRef{{ID: rootFiles[0].FileID}}, mk.FileID, ""); err != nil {
		t.Fatalf("copy: %v", err)
	}
	subFiles, _ := drive.ListDir(uid, driveID, mk.FileID, nil)
	if !hasFile(subFiles, "renamed.txt") {
		t.Fatalf("copy failed: %+v", names(subFiles))
	}

	// 10. delete both
	if _, err := drive.DeleteBatch(uid, driveID, []drive.FileRef{{ID: rootFiles[0].FileID}, {ID: subFiles[0].FileID}}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rootFiles, _ = drive.ListDir(uid, driveID, "/", nil)
	subFiles, _ = drive.ListDir(uid, driveID, mk.FileID, nil)
	if hasFile(rootFiles, "renamed.txt") || hasFile(subFiles, "renamed.txt") {
		t.Fatalf("delete incomplete: root=%v sub=%v", names(rootFiles), names(subFiles))
	}
}

func hasFile(files []model.File, name string) bool {
	for _, f := range files {
		if f.Name == name {
			return true
		}
	}
	return false
}

func names(files []model.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}