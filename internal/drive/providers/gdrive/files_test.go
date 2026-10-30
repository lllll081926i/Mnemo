package gdrive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
)

func TestPagedListsPreserveShortcutIDAndSharedScope(t *testing.T) {
	c := account()
	c.Token.ProviderSpaceID = "shared-drive"
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.URL.Path == "/drive/v3/files/root-id" {
			return response(r, 200, `{"id":"root-id","mimeType":"application/vnd.google-apps.folder"}`), nil
		}
		if q.Get("supportsAllDrives") != "true" || q.Get("includeItemsFromAllDrives") != "true" || q.Get("driveId") != "shared-drive" || q.Get("corpora") != "drive" {
			t.Fatalf("共享盘参数不完整：%v", q)
		}
		if q.Get("pageToken") == "" {
			return response(r, 200, `{"files":[{"id":"shortcut","name":"目录快捷方式","mimeType":"application/vnd.google-apps.shortcut","parents":["root-id"],"shortcutDetails":{"targetId":"original","targetMimeType":"application/vnd.google-apps.folder"}}],"nextPageToken":"second"}`), nil
		}
		if q.Get("pageToken") != "second" {
			t.Fatal("分页游标丢失")
		}
		return response(r, 200, `{"files":[{"id":"file","name":"文档.pdf","mimeType":"application/pdf","size":"3","md5Checksum":"abc","parents":["root-id"]}]}`), nil
	})
	files, err := (&Driver{}).List(t.Context(), c, "root", nil)
	if err != nil || len(files) != 2 || files[0].FileID != "shortcut" || !files[0].IsDir || files[1].ContentHash != "abc" {
		t.Fatalf("目录映射失败：%+v %v", files, err)
	}
}

func TestIncompleteOrRepeatedPaginationFailsWithoutPartialResult(t *testing.T) {
	for _, body := range []string{`{"files":[],"incompleteSearch":true}`, `{"nextPageToken":"x"}`, `{"files":[],"nextPageToken":"x"}`} {
		t.Run(body, func(t *testing.T) {
			useTransport(t, func(r *http.Request) (*http.Response, error) { return response(r, 200, body), nil })
			files, err := (&Driver{}).ListTrash(t.Context(), account(), nil)
			if err == nil || files != nil {
				t.Fatalf("将部分结果作为成功返回：%+v %v", files, err)
			}
		})
	}
}

func TestSearchEscapesQuery(t *testing.T) {
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("q") != `trashed=false and name contains 'a\'b\\c'` {
			t.Fatalf("搜索未正确转义：%s", r.URL.Query().Get("q"))
		}
		return response(r, 200, `{"files":[]}`), nil
	})
	if _, err := (&Driver{}).Search(t.Context(), account(), `a'b\c`); err != nil {
		t.Fatal(err)
	}
}

func TestTrashRestoreAndPermanentDeleteUseSelectedShortcut(t *testing.T) {
	requests := 0
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/drive/v3/files/shortcut" {
			t.Fatal("错误地操作快捷方式目标")
		}
		if r.Method == http.MethodDelete {
			return response(r, 204, ""), nil
		}
		if r.Method != http.MethodPatch {
			t.Fatal("回收站使用了错误方法")
		}
		var body map[string]bool
		_ = json.NewDecoder(r.Body).Decode(&body)
		value, exists := body["trashed"]
		if !exists {
			t.Fatal("未声明回收站状态")
		}
		encoded, _ := json.Marshal(map[string]any{"id": "shortcut", "trashed": value})
		return response(r, 200, string(encoded)), nil
	})
	d := &Driver{}
	c := account()
	if _, err := d.Trash(t.Context(), c, []string{"shortcut"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Restore(t.Context(), c, []string{"shortcut"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Delete(t.Context(), c, []drive.FileRef{{ID: "shortcut"}}); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("意外请求数：%d", requests)
	}
}

func TestMoveUsesActualSourceParent(t *testing.T) {
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/drive/v3/files/destination":
			return response(r, 200, `{"id":"destination","mimeType":"application/vnd.google-apps.folder"}`), nil
		case "/drive/v3/files/file":
			if r.Method == http.MethodGet {
				return response(r, 200, `{"id":"file","name":"name","parents":["actual-parent"]}`), nil
			}
			if r.URL.Query().Get("removeParents") != "actual-parent" || r.URL.Query().Get("addParents") != "destination" {
				t.Fatal("移动硬编码根目录")
			}
			return response(r, 200, `{"id":"file","parents":["destination"]}`), nil
		}
		t.Fatalf("意外请求 %s", r.URL.Path)
		return nil, nil
	})
	if _, err := (&Driver{}).Move(t.Context(), account(), []drive.FileRef{{ID: "file"}}, "destination", ""); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadKeepsAuthInHeadersAndRejectsNativeDocuments(t *testing.T) {
	for _, test := range []struct {
		body string
		ok   bool
	}{
		{`{"id":"file","name":"file.pdf","mimeType":"application/pdf","size":"3","capabilities":{"canDownload":true}}`, true},
		{`{"id":"file","name":"doc","mimeType":"application/vnd.google-apps.document"}`, false},
		{`{"id":"file","name":"file.pdf","mimeType":"application/pdf","size":"3","capabilities":{"canDownload":false}}`, false},
		{`{"id":"file","name":"file.pdf","mimeType":"application/pdf"}`, false},
	} {
		t.Run(test.body, func(t *testing.T) {
			useTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.oplist.org" {
					return response(r, 200, `{"access_token":"fresh","refresh_token":"refresh"}`), nil
				}
				return response(r, 200, test.body), nil
			})
			source, err := (&Driver{}).GetDownloadURL(t.Context(), account(), "file", 0)
			if (err == nil) != test.ok {
				t.Fatalf("下载能力判断错误：%v", err)
			}
			if test.ok && (strings.Contains(source.URL, "access") || source.Headers["Authorization"] != "Bearer fresh" || source.RequestAuth == nil || !source.ForceLocalProxy || source.Size != 3) {
				t.Fatalf("下载源未保留鉴权：%+v", source)
			}
			if test.ok {
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, source.URL, nil)
				if err := source.RequestAuth(req); err != nil || req.Header.Get("Authorization") != "Bearer fresh" {
					t.Fatalf("请求鉴权失败：%v", err)
				}
				foreign, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://untrusted.test/file", nil)
				if err := source.RequestAuth(foreign); err == nil || foreign.Header.Get("Authorization") != "" {
					t.Fatal("下载鉴权流向其他主机")
				}
			}
		})
	}
}

func TestFavoritesReadAndWriteAndCancellation(t *testing.T) {
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			if r.URL.Query().Get("q") != "trashed=false and starred=true" {
				t.Fatal("收藏列表查询错误")
			}
			return response(r, 200, `{"files":[]}`), nil
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"starred":true`) {
			t.Fatal("收藏修改参数错误")
		}
		return response(r, 200, `{"id":"file","starred":true}`), nil
	})
	d := &Driver{}
	if _, err := d.ListFavorites(t.Context(), account()); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Favorite(t.Context(), account(), []string{"file"}, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Trash(ctx, account(), []string{"file"}); err == nil {
		t.Fatal("忽略取消")
	}
}
