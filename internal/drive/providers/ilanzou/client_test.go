package ilanzou

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func withNoThrottle(t *testing.T) {
	t.Helper()
	old := fetchMinInterval
	fetchMinInterval = 0
	t.Cleanup(func() { fetchMinInterval = old })
}

// TestFileListPagination drives /record/file/list against a fake API and
// verifies the offset paging loop stops at totalPage.
func TestFileListPagination(t *testing.T) {
	withNoThrottle(t)
	oldBase := ILANZOU_CONF.Base
	t.Cleanup(func() { ILANZOU_CONF.Base = oldBase })

	var mu sync.Mutex
	var offsets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		mu.Lock()
		offsets = append(offsets, offset)
		mu.Unlock()
		if offset == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"list": []any{
					map[string]any{"fileType": 2, "folderId": 9, "folderName": "dir"},
					map[string]any{"fileType": 0, "fileId": 8, "fileName": "a.zip", "fileSize": 2},
				},
				"totalPage": 2,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "list": []any{}, "totalPage": 2})
	}))
	defer srv.Close()

	d := &Driver{}
	c := drive.Context{DriveID: "ilanzou:u", Token: &model.TokenInfo{RefreshToken: `{}`}}
	// point the API at the fake server via the package config
	ILANZOU_CONF.Base = srv.URL

	items, err := d.fileList(context.Background(), c, "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].FolderName != "dir" || items[1].FileName != "a.zip" {
		t.Errorf("items wrong: %+v", items)
	}
	if len(offsets) != 2 || offsets[0] != "1" || offsets[1] != "2" {
		t.Errorf("offset sequence = %v, want [1 2]", offsets)
	}
	// pagination must not request beyond totalPage
	for _, o := range offsets {
		if o == "3" {
			t.Errorf("requested beyond totalPage: %v", offsets)
		}
	}
}

// TestLoginAndAccountMap drives the /login + /user/account/map flow.
func TestLoginAndAccountMap(t *testing.T) {
	withNoThrottle(t)
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/getUuid"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "uuid": "server-uuid-123"})
		case strings.HasSuffix(r.URL.Path, "/login"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["loginName"] != "user" || body["loginPwd"] != "pass" {
				http.Error(w, "bad credentials", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{"appToken": "tok-123"},
			})
		case strings.HasSuffix(r.URL.Path, "/user/account/map"):
			if r.URL.Query().Get("appToken") != "tok-123" {
				http.Error(w, "missing appToken", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"map":  map[string]any{"userId": "42", "account": "user@ilanzou"},
			})
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	old := ILANZOU_CONF.Base
	ILANZOU_CONF.Base = srv.URL
	defer func() { ILANZOU_CONF.Base = old }()

	login, err := ilanzouLogin(context.Background(), "user", "pass", "")
	if err != nil {
		t.Fatal(err)
	}
	if login.token != "tok-123" {
		t.Errorf("token = %q, want tok-123", login.token)
	}
	if login.uuid != "server-uuid-123" {
		t.Errorf("uuid = %q, want server-uuid-123", login.uuid)
	}
	if login.userId != "42" || login.account != "user@ilanzou" {
		t.Errorf("map = %+v", login)
	}
	if len(calls) != 3 {
		t.Errorf("calls = %v, want 3", calls)
	}
}
