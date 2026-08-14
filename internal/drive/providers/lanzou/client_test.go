package lanzou

import (
	"context"
	"encoding/json"
	"fmt"
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

func testCtx(driveID, baseURL, cookie string) drive.Context {
	return drive.Context{
		DriveID: driveID,
		Token: &model.TokenInfo{
			AccessToken:  cookie,
			RefreshToken: fmt.Sprintf(`{"type":"cookie","cookie":%q,"baseUrl":%q}`, cookie, baseURL),
		},
	}
}

// TestFileListPagination exercises the task-47 / task-5 paging loop and the
// folder+file mapping.
func TestFileListPagination(t *testing.T) {
	withNoThrottle(t)
	var mu sync.Mutex
	var pgs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("task") {
		case "47":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"zt": 1, "text": []any{map[string]any{"fol_id": "9", "name": "dir"}},
			})
		case "5":
			pg := r.Form.Get("pg")
			mu.Lock()
			pgs = append(pgs, pg)
			mu.Unlock()
			if pg == "1" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"zt": 1, "text": []any{map[string]any{"id": "8", "name_all": "a.zip", "size": "2M"}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1, "text": []any{}})
		default:
			http.Error(w, "unknown task", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	d := &Driver{}
	c := testCtx("lanzou:u", srv.URL, "cookie1")
	items, err := d.fileList(context.Background(), c, "-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "dir" || items[0].FolID != "9" {
		t.Errorf("folder item wrong: %+v", items[0])
	}
	if items[1].NameAll != "a.zip" || items[1].ID != "8" {
		t.Errorf("file item wrong: %+v", items[1])
	}
	// pages 1 and 2 requested, loop stopped on the empty page
	if len(pgs) != 2 || pgs[0] != "1" || pgs[1] != "2" {
		t.Errorf("paging sequence = %v, want [1 2]", pgs)
	}
}

// TestFetchTextAcwRetry verifies that a challenge page is solved and the acw
// cookie is re-sent on the next attempt.
func TestFetchTextAcwRetry(t *testing.T) {
	withNoThrottle(t)
	attempts := 0
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>var arg1='00112233445566778899AABBCCDDEEFF00112233';</script>`))
			return
		}
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := fetchText(context.Background(), http.MethodGet, srv.URL, nil, nil, "base=1", false)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if res.text != "ok" {
		t.Fatalf("final text = %q, want ok", res.text)
	}
	if !strings.Contains(gotCookie, "acw_sc__v2=41eb1062441a5dadc03039c05aff6731a59d0124") {
		t.Errorf("acw cookie missing from %q", gotCookie)
	}
	if !strings.Contains(gotCookie, "base=1") {
		t.Errorf("original cookie lost in %q", gotCookie)
	}
}
