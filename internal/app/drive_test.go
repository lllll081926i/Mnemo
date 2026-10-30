package app

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer"
)

func TestPikPakOfflineDownloadDispatchesAndPersists(t *testing.T) {
	registration := drive.Require(model.ProviderPikpak)
	if !registration.Caps.OfflineDownload {
		t.Fatal("PikPak 必须声明云离线能力")
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const userID = "pikpak_offline-test"
	const driveID = "offline-drive"
	const magnet = "magnet:?xt=urn:btih:0123456789012345678901234567890123456789"
	drive.SetTokenResolver(func(user, space string) (*model.TokenInfo, error) {
		if user != userID || space != driveID {
			t.Fatalf("错误的离线账号: %s, %s", user, space)
		}
		return &model.TokenInfo{AccessToken: "offline-token", DeviceID: "offline-device", UserID: userID}, nil
	})
	previous := netx.TestTransportHook
	t.Cleanup(func() {
		netx.TestTransportHook = previous
		drive.SetTokenResolver(nil)
	})
	requests := 0
	netx.TestTransportHook = favoriteTestTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodPost || req.URL.Host != "api-drive.mypikpak.com" || req.URL.Path != "/drive/v1/files" {
			t.Fatalf("意外的离线请求: %s %s", req.Method, req.URL)
		}
		var body struct {
			Kind       string            `json:"kind"`
			UploadType string            `json:"upload_type"`
			FolderType string            `json:"folder_type"`
			Name       string            `json:"name"`
			URL        map[string]string `json:"url"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if req.Header.Get("Authorization") != "Bearer offline-token" || body.Kind != "drive#file" || body.UploadType != "UPLOAD_TYPE_URL" || body.FolderType != "DOWNLOAD" || body.Name != "测试磁力" || body.URL["url"] != magnet {
			t.Fatalf("错误的离线请求参数: %+v", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"task":{"task_id":"offline-task"},"file":{"id":"offline-file"}}`)), Request: req}, nil
	})
	app := &App{store: st}
	task, err := app.OfflineDownload(userID, driveID, magnet, "测试磁力")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || task.ID != "offline-task" || task.FileID != "offline-file" || task.Status != "running" || task.UserID != userID || task.DriveID != driveID || task.URL != magnet {
		t.Fatalf("错误的离线任务: %+v，请求次数: %d", task, requests)
	}
	saved := app.ListOfflineTasks(userID)
	if len(saved) != 1 || saved[0] != *task {
		t.Fatalf("离线任务没有正确保存: %+v", saved)
	}
}

func TestThunderOfflineApplicationFlow(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const userID = "thunder_offline-app"
	const driveID = "offline-app"
	drive.SetTokenResolver(func(user, space string) (*model.TokenInfo, error) {
		if user != userID || space != driveID {
			t.Fatal("迅雷离线任务调用了错误账号")
		}
		return &model.TokenInfo{AccessToken: "thunder-access", RefreshToken: `{"refresh_token":"thunder-refresh","user_id":"offline-app","device_id":"0123456789abcdef0123456789abcdef"}`}, nil
	})
	previous := netx.TestTransportHook
	t.Cleanup(func() { drive.SetTokenResolver(nil); netx.TestTransportHook = previous })
	deleted := false
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api-pan.xunlei.com" || request.Header.Get("Authorization") != "Bearer thunder-access" {
			t.Fatal("迅雷请求使用了错误的服务或凭据")
		}
		body := `{"task":{"id":"thunder-task","file_id":"thunder-file"}}`
		if request.Method == http.MethodGet {
			body = `{"tasks":[{"id":"thunder-task","file_id":"thunder-file","phase":"PHASE_TYPE_COMPLETE","file_size":"1024","progress":100}]}`
		} else if request.Method == http.MethodDelete {
			deleted = request.URL.Query().Get("task_ids") == "thunder-task" && request.URL.Query().Get("delete_files") == "false"
			body = `{}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	app := &App{store: st}
	task, err := app.OfflineDownload(userID, driveID, "magnet:?xt=urn:btih:test", "测试")
	if err != nil || task.ID != "thunder-task" {
		t.Fatalf("应用未能调用迅雷离线创建: %+v, %v", task, err)
	}
	tasks, err := app.RefreshOfflineTasks(userID, driveID)
	if err != nil || len(tasks) != 1 || tasks[0].Status != "PHASE_TYPE_COMPLETE" || tasks[0].FileSize != 1024 {
		t.Fatalf("应用未能刷新迅雷任务: %+v, %v", tasks, err)
	}
	if err := app.DeleteOfflineTask(userID, driveID, task.ID, false); err != nil || !deleted {
		t.Fatalf("应用未能删除迅雷任务: %v", err)
	}
	if tasks := app.ListOfflineTasks(userID); len(tasks) != 0 {
		t.Fatalf("已删除的迅雷任务仍在本地: %+v", tasks)
	}
}

func TestOfflineCapabilitiesReachFrontend(testContext *testing.T) {
	providers := (&App{}).ListProviders()
	if len(providers) != drive.Count() {
		testContext.Fatal("前端网盘列表遗漏注册驱动")
	}
	for _, provider := range providers {
		_, implemented := drive.Require(provider.ID).Factory().(drive.OfflineDriver)
		if provider.Capabilities.OfflineDownload != implemented {
			testContext.Errorf("%s 的前端云离线开关=%v，完整实现=%v", provider.ID, provider.Capabilities.OfflineDownload, implemented)
		}
	}
}

func TestFavoritesBackupListsLocalRecordsWithoutNetwork(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddFavorite(store.Favorite{UserID: "pikpak:old", DriveID: "old", FileID: "file", Name: "photo.jpg"}); err != nil {
		t.Fatal(err)
	}
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		t.Fatal("backup must not load account credentials")
		return nil, nil
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil) })
	got, err := (&App{store: st}).ListFavorites("", "")
	if err != nil || len(got) != 1 {
		t.Fatalf("backup favorites = %+v, %v", got, err)
	}
}

type favoriteTestTransport func(*http.Request) (*http.Response, error)

func (f favoriteTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFavoritesCloudFailureDoesNotChangeLocalRecords(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const provider = "onedrive"
	status := http.StatusForbidden
	old := netx.TestTransportHook
	netx.TestTransportHook = favoriteTestTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"error":{"code":"accessDenied","message":"cloud unavailable"}}`
		if status == http.StatusOK {
			body = `{"id":"file"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		return &model.TokenInfo{AccessToken: "token", ProviderDriveType: "business"}, nil
	})
	t.Cleanup(func() { netx.TestTransportHook = old; drive.SetTokenResolver(nil) })
	user, space := provider+"_user", provider
	f := store.Favorite{UserID: user, DriveID: space, FileID: "file", Name: "old.jpg", Source: "cloud"}
	if err := st.AddFavorite(f); err != nil {
		t.Fatal(err)
	}
	a := &App{store: st}
	if _, err := a.ListFavorites(user, space); err == nil {
		t.Fatal("list error hidden")
	}
	if err := a.RemoveFavorite(user, space, "file"); err == nil {
		t.Fatal("remove error hidden")
	}
	f.Name = "new.jpg"
	if err := a.AddFavorite(user, space, f); err == nil {
		t.Fatal("add error hidden")
	}
	got, err := st.ListFavorites(user, space)
	if err != nil || len(got) != 1 || got[0].Name != "old.jpg" {
		t.Fatalf("cloud failure changed cache: %+v, %v", got, err)
	}
	status = http.StatusOK
	f.File = &model.File{FileID: "other", Name: "old.jpg", DownloadURL: "https://secret.invalid/signed", Size: 42, ParentFileID: "parent"}
	if err := a.AddFavorite(user, space, f); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListFavorites(user, space)
	if got[0].Source != "cloud" || got[0].File.FileID != "file" || got[0].File.DownloadURL != "" || got[0].File.Size != 42 {
		t.Fatalf("invalid stored snapshot: %+v", got)
	}
	if err := a.RemoveFavorite(user, space, "file"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListFavorites(user, space)
	if len(got) != 0 {
		t.Fatal("removed favorite remains")
	}
}

func TestFavoritesLocalFallbackForEveryUnsupportedProvider(t *testing.T) {
	old := netx.TestTransportHook
	netx.TestTransportHook = favoriteTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("local favorites made a network request: %s", r.URL)
		return nil, nil
	})
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		return &model.TokenInfo{ProviderDriveType: "personal"}, nil
	})
	t.Cleanup(func() { netx.TestTransportHook = old; drive.SetTokenResolver(nil) })
	for _, provider := range []string{"onedrive", "dropbox", "lanzou", "ilanzou", "pan139", "pan189", "guangya", "webdav", "s3"} {
		t.Run(provider, func(t *testing.T) {
			st, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			user := provider + "_local"
			if provider == "webdav" || provider == "s3" {
				user = provider + ":local"
			}
			a := &App{store: st}
			f := store.Favorite{FileID: "file", Name: "movie.mp4", File: &model.File{Size: 42, ParentFileID: "folder"}}
			if err := a.AddFavorite(user, "space", f); err != nil {
				t.Fatal(err)
			}
			list, err := a.ListFavorites(user, "space")
			if err != nil || len(list) != 1 || list[0].Source != "local" || list[0].File.Size != 42 {
				t.Fatalf("local favorites=%+v, %v", list, err)
			}
			if err := a.RemoveFavorite(user, "space", "file"); err != nil {
				t.Fatal(err)
			}
			list, err = a.ListFavorites(user, "space")
			if err != nil || len(list) != 0 {
				t.Fatalf("removed favorite remains: %+v %v", list, err)
			}
		})
	}
}

func TestRestoreFavoriteNeverReplaysCloudChangesOrOverwritesCurrentRecord(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		t.Fatal("restore read account credentials")
		return nil, nil
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil) })
	a := &App{store: st}
	f := store.Favorite{UserID: "pikpak_removed", DriveID: "d", FileID: "file", Name: "old.jpg", Source: "cloud", Added: 25, File: &model.File{DownloadURL: "https://secret.invalid/signed", Thumbnail: "https://secret.invalid/thumb", Size: 42}}
	if err := a.RestoreFavorite(f); err != nil {
		t.Fatal(err)
	}
	got, _ := st.ListFavorites(f.UserID, f.DriveID)
	if len(got) != 1 || got[0].Source != "local" || got[0].Added != 25 || got[0].File.DownloadURL != "" || got[0].File.Thumbnail != "" {
		t.Fatalf("unsafe restored favorite: %+v", got)
	}
	if err := a.RemoveFavorite(f.UserID, f.DriveID, f.FileID); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListFavorites(f.UserID, f.DriveID)
	if len(got) != 0 {
		t.Fatal("restored local favorite was not removed")
	}
	f.Name, f.Source = "current.jpg", "cloud"
	if err := st.AddFavorite(f); err != nil {
		t.Fatal(err)
	}
	f.Name, f.Source = "backup.jpg", "local"
	if err := a.RestoreFavorite(f); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListFavorites(f.UserID, f.DriveID)
	if got[0].Name != "current.jpg" || got[0].Source != "cloud" {
		t.Fatal("backup overwrote current favorite")
	}
}

func TestSettingsReadFailureReachesCaller(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{store: st}
	if _, err := a.GetSettings(); err == nil {
		t.Fatal("corrupt settings must return an error rather than defaults")
	}
}

func TestSettingsRejectInvalidDownloadDirectoryWithoutPersisting(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	previous := store.DefaultSettings()
	previous.DownloadDir = filepath.Join(dir, "downloads")
	if err := st.SetSettings(previous); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := previous
	changed.DownloadDir = blocked
	if err := (&App{store: st}).SaveSettings(changed); err == nil {
		t.Fatal("saving a regular file as the download directory must fail")
	}
	actual, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if actual.DownloadDir != previous.DownloadDir {
		t.Fatalf("failed validation replaced settings: %q", actual.DownloadDir)
	}
}

func TestSettingsNormalizeDownloadDirectoryAndRestoreDefault(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("MNEMO_DOWNLOAD_TEST_ROOT", dir)
	m, err := transfer.NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	a := &App{store: st, dl: m}
	s := store.DefaultSettings()
	s.DownloadDir = ` "%MNEMO_DOWNLOAD_TEST_ROOT%/中文 下载" `
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.GetSettings()
	want := filepath.Join(dir, "中文 下载")
	if saved.DownloadDir != want {
		t.Fatalf("saved directory = %q, want %q", saved.DownloadDir, want)
	}
	if got, err := a.GetDownloadDirectory(); err != nil || got != want {
		t.Fatalf("runtime directory = %q, %v", got, err)
	}
	saved.DownloadDir = ""
	if err := a.SaveSettings(saved); err != nil {
		t.Fatal(err)
	}
	wantDefault, err := config.DefaultDownloadDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.GetDownloadDirectory(); err != nil || got != wantDefault {
		t.Fatalf("restored runtime directory = %q, %v", got, err)
	}
	saved, _ = st.GetSettings()
	if saved.DownloadDir != "" {
		t.Fatal("system default was hardcoded into settings")
	}
}

func TestShouldPersistShareHistory(t *testing.T) {
	if shouldPersistShareHistory(&model.ShareItem{SharePolicy: "presigned"}) {
		t.Fatal("presigned URL must not be persisted as share history")
	}
	if !shouldPersistShareHistory(&model.ShareItem{SharePolicy: "public"}) {
		t.Fatal("provider-managed share should be persisted")
	}
}

func TestValidateShareRecordProvider(t *testing.T) {
	if err := validateShareRecordProvider(model.ShareHistoryEntry{}, model.ProviderDropbox); err != nil {
		t.Fatalf("legacy share record should remain usable: %v", err)
	}
	if err := validateShareRecordProvider(model.ShareHistoryEntry{Provider: model.ProviderDropbox}, model.ProviderDropbox); err != nil {
		t.Fatalf("matching provider should be accepted: %v", err)
	}
	if err := validateShareRecordProvider(model.ShareHistoryEntry{Provider: model.ProviderDropbox}, model.ProviderOnedrive); err == nil {
		t.Fatal("mismatched provider must be rejected before cancellation")
	}
}
