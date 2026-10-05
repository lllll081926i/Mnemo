package transfer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestDownloadFailureLogsConcreteErrorWithoutCredentials(t *testing.T) {
	root := t.TempDir()
	if err := logging.Configure(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logging.Close)
	logPath := logging.Path()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(st, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	task := &model.DownloadTask{ID: "diagnostic-task", URL: server.URL + "/resource?signature=private-signature", Name: "file.bin", Status: "queued", Size: 77}
	if err := manager.addDownloadTask(task, task.Name); err != nil {
		t.Fatal(err)
	}
	manager.runDownload(task)
	if task.Status != "failed" || !strings.Contains(task.Error, "probe http 404") {
		t.Fatalf("下载具体错误丢失: %+v", task)
	}
	logging.Close()
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(content)
	for _, field := range []string{"[WARN] download worker failed", "task_id=diagnostic-task", "probe http 404", "downloaded=0", "total=77", "configured_connections="} {
		if !strings.Contains(output, field) {
			t.Errorf("下载日志缺少 %s: %s", field, output)
		}
	}
	if strings.Contains(output, "private-signature") {
		t.Fatal("下载日志泄露直链签名")
	}
}
