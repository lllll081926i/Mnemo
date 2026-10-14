package app

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func TestOfflineRemoteSuccessNotDowngradedByLocalPersistenceFailure(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Force a write error after the provider accepts the task.
	if err := os.Mkdir(filepath.Join(st.Dir(), "offline.json.tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		return &model.TokenInfo{TokenFrom: "pikpak", AccessToken: "token", DeviceID: "device"}, nil
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil) })
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	calls := 0
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/drive/v1/files" {
			t.Fatalf("unexpected cloud task request: %v", request.URL)
		}
		body := `{"task":{"task_id":"remote-task"},"file":{"id":"remote-file"}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	app := NewApp()
	app.store = st
	task, err := app.OfflineDownload("pikpak_test", "drive", "magnet:?xt=urn:btih:abc", "sample")
	if err != nil || task == nil || task.ID != "remote-task" || calls != 1 {
		t.Fatalf("remote submission was lost or retried: task=%+v err=%v calls=%d", task, err, calls)
	}
	if entries := app.ListOfflineTasks("pikpak_test"); len(entries) != 0 {
		t.Fatalf("local persistence failure was falsely recorded: %+v", entries)
	}
}
