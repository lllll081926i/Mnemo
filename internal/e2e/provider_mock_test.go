package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestPikPakListMock(t *testing.T) {
	// Mock pikpak drive API (api-drive.mypikpak.com)
	mock := MockAPI(t, "api-drive.mypikpak.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/drive/v1/files" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{"id": "1", "name": "video.mp4", "kind": "drive#file", "size": 1000, "parent_id": "root", "thumbnail_link": "", "modified_time": "2026-01-01T00:00:00Z", "web_content_link": ""},
					{"id": "2", "name": "folder", "kind": "drive#folder", "size": 0, "parent_id": "root", "modified_time": "2026-01-01T00:00:00Z", "web_content_link": ""},
				},
				"next_page_token": "",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "pikpak", &model.TokenInfo{
		AccessToken: "test-token", DeviceID: "dev", TokenFrom: "pikpak",
		RefreshToken: "test", ExpiresIn: 3600, UserID: "pikpak_test",
	})

	names := listNames(t, uid, did, "pikpak_root")
	if len(names) != 2 || names[0] != "video.mp4" || names[1] != "folder" {
		t.Fatalf("names: %v", names)
	}

	// Verify driver resolves correctly
	provider := drive.ProviderOf(uid, did, "")
	if provider != "pikpak" {
		t.Fatalf("provider: %s", provider)
	}
}

func TestPikPakDownloadURL(t *testing.T) {
	mock := MockAPI(t, "api-drive.mypikpak.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/drive/v1/files/1/download" {
			json.NewEncoder(w).Encode(map[string]any{"url": "https://dl.example.com/file", "size": 1000})
			return
		}
		if r.URL.Path == "/drive/v1/files/1" {
			json.NewEncoder(w).Encode(map[string]any{"id": "1", "name": "v.mp4", "kind": "drive#file", "size": 1000, "parent_id": "root", "web_content_link": "https://dl.example.com/file", "modified_time": time.Now().Format(time.RFC3339)})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "pikpak", &model.TokenInfo{
		AccessToken: "test-token", TokenFrom: "pikpak", UserID: "pikpak_test",
	})

	dl, err := drive.GetDownloadURL(uid, did, "1", 3600)
	if err != nil {
		t.Fatalf("GetDownloadURL: %v", err)
	}
	if dl.URL == "" {
		t.Fatal("empty download URL")
	}
	if dl.Size != 1000 {
		t.Fatalf("size: %d", dl.Size)
	}
}

func TestOneDriveListMock(t *testing.T) {
	mock := MockAPI(t, "graph.microsoft.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/me/drive/root/children" {
			json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "1", "name": "doc.txt", "size": 500, "file": map[string]any{"mimeType": "text/plain"}, "lastModifiedDateTime": "2026-01-01T00:00:00Z", "parentReference": map[string]any{"id": "root"}},
					{"id": "2", "name": "pics", "folder": map[string]any{}, "lastModifiedDateTime": "2026-01-01T00:00:00Z", "parentReference": map[string]any{"id": "root"}},
				},
				"@odata.nextLink": nil,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "onedrive", &model.TokenInfo{
		AccessToken: "test-token", TokenFrom: "onedrive", UserID: "onedrive_test",
	})

	names := listNames(t, uid, did, "onedrive_root")
	if len(names) != 2 || names[0] != "doc.txt" || names[1] != "pics" {
		t.Fatalf("names: %v", names)
	}
}

func TestOneDriveDownloadURL(t *testing.T) {
	mock := MockAPI(t, "graph.microsoft.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/me/drive/items/1" {
			json.NewEncoder(w).Encode(map[string]any{
				"id": "1", "name": "doc.txt", "size": 500, "file": map[string]any{"mimeType": "text/plain"},
				"@microsoft.graph.downloadUrl": "https://dl.example.com/doc",
				"lastModifiedDateTime": "2026-01-01T00:00:00Z", "parentReference": map[string]any{"id": "root"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "onedrive", &model.TokenInfo{
		AccessToken: "test-token", TokenFrom: "onedrive", UserID: "onedrive_test",
	})

	dl, err := drive.GetDownloadURL(uid, did, "1", 3600)
	if err != nil {
		t.Fatalf("GetDownloadURL: %v", err)
	}
	if dl.URL == "" {
		t.Fatal("empty download URL")
	}
}

func TestDropboxListMock(t *testing.T) {
	mock := MockAPI(t, "api.dropboxapi.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2/files/list_folder" {
			json.NewEncoder(w).Encode(map[string]any{
				"entries": []map[string]any{
					{".tag": "file", "id": "id:1", "name": "note.txt", "path_display": "/note.txt", "size": 300, "server_modified": "2026-01-01T00:00:00Z"},
					{".tag": "folder", "id": "id:2", "name": "data", "path_display": "/data", "size": 0, "server_modified": "2026-01-01T00:00:00Z"},
				},
				"cursor": "", "has_more": false,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "dropbox", &model.TokenInfo{
		AccessToken: "test-token", TokenFrom: "dropbox", UserID: "dropbox_test",
	})

	names := listNames(t, uid, did, "dropbox_root")
	if len(names) != 2 || names[0] != "note.txt" || names[1] != "data" {
		t.Fatalf("names: %v", names)
	}
}

func TestAliopenListMock(t *testing.T) {
	session := map[string]any{
		"access_token": "test-token", "refresh_token": "test-refresh",
		"drive_id": "d1", "resource_drive_id": "d2",
	}
	sessionJSON, _ := json.Marshal(session)
	mock := MockAPI(t, "openapi.alipan.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/adrive/v1.0/openFile/list":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"file_id": "f1", "name": "report.pdf", "parent_file_id": "root", "type": "file", "size": 2000, "updated_at": "2026-01-01T00:00:00Z", "content_hash": "", "thumbnail": "", "category": "doc"},
					{"file_id": "f2", "name": "images", "parent_file_id": "root", "type": "folder", "size": 0, "updated_at": "2026-01-01T00:00:00Z", "content_hash": "", "thumbnail": ""},
				},
				"next_marker": "",
			})
		default:
			// For getDriveInfo during boot
			json.NewEncoder(w).Encode(map[string]any{"default_drive_id": "d1", "resource_drive_id": "d2"})
		}
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "aliopen", &model.TokenInfo{
		AccessToken: "test-token", RefreshToken: string(sessionJSON),
		TokenFrom: "aliopen", UserID: "aliopen_test",
	})

	// list on backup root (will show virtual backup/resource dirs first)
	names := listNames(t, uid, did, "aliopen_root")
	_ = names
	// first call returns virtual dirs, not the API list
	if len(names) != 2 || names[0] != "备份盘" {
		t.Fatalf("root names: %v", names)
	}
}