package pikpak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/netx"
)

func TestPikPakSystemStarTagOverridesFalseBoolean(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{"tag with false starred", `{"id":"one","name":"movie.mkv","starred":false,"tags":[{"name":"STAR","id":"tag"}]}`, true},
		{"legacy boolean", `{"id":"two","name":"movie.mkv","starred":true}`, true},
		{"unstarred", `{"id":"three","name":"movie.mkv","starred":false,"tags":[{"name":"SOMETHING_ELSE"}]}`, false},
		{"case insensitive tag", `{"id":"four","name":"folder","kind":"drive#folder","tags":[{"name":"star"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file File
			if err := json.Unmarshal([]byte(tc.raw), &file); err != nil {
				t.Fatal(err)
			}
			got := mapFile(&file, "drive", "root")
			if got.Starred != tc.want {
				t.Fatalf("mapped starred=%v, want %v for %s", got.Starred, tc.want, tc.raw)
			}
		})
	}
}

func TestPikPakFavoritesUsesStarredFilterWithoutPhaseRestriction(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	calls := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/drive/v1/files" {
			return nil, fmt.Errorf("unexpected favorites request: %s %s", req.Method, req.URL)
		}
		calls++
		filter := req.URL.Query().Get("filters")
		if !strings.Contains(filter, `"starred":{"eq":true}`) || !strings.Contains(filter, `"trashed":{"eq":false}`) || strings.Contains(filter, "phase") {
			t.Errorf("incorrect favorites filter: %s", filter)
		}
		if req.URL.Query().Get("parent_id") != "*" {
			t.Errorf("favorite listing is not drive-wide")
		}
		if calls == 1 {
			return pikpakResponse(req, 200, `{"files":[{"id":"folder-1","name":"Folder","kind":"drive#folder","starred":false,"tags":[{"name":"STAR"}]}],"next_page_token":"next"}`), nil
		}
		if req.URL.Query().Get("page_token") != "next" {
			t.Errorf("favorite cursor missing")
		}
		return pikpakResponse(req, 200, `{"files":[{"id":"item-2","name":"Pending","phase":"PHASE_TYPE_RUNNING"}]}`), nil
	})
	cl := newClient("access", "device", "account")
	result, err := cl.ListFavorites(context.Background())
	if err != nil || len(result) != 2 || calls != 2 {
		t.Fatalf("favorites: %+v, %v, calls=%d", result, err, calls)
	}
	if !mapFile(&result[0], "drive", "root").Starred {
		t.Fatal("STAR tag in favorites not recognized")
	}
}

func TestPikPakStarAndUnstarUseBatchEndpoints(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	operations := 0
	netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || (req.URL.Path != "/drive/v1/files:star" && req.URL.Path != "/drive/v1/files:unstar") {
			return nil, fmt.Errorf("unexpected star request %s %s", req.Method, req.URL)
		}
		operations++
		var b struct { IDs []string `json:"ids"` }
		if err := json.NewDecoder(req.Body).Decode(&b); err != nil { return nil, err }
		if len(b.IDs) != 1 || b.IDs[0] != "file-id" { t.Errorf("star ids: %+v", b.IDs) }
		return pikpakResponse(req, 200, `{}`), nil
	})
	cl := newClient("access", "device", "account")
	if err := cl.Star(t.Context(), []string{"file-id"}, true); err != nil { t.Fatal(err) }
	if err := cl.Star(t.Context(), []string{"file-id"}, false); err != nil { t.Fatal(err) }
	if operations != 2 { t.Fatalf("star operations=%d", operations) }
}
