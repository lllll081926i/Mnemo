package drive

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/model"
)

func assertFileMetaBounds(t *testing.T, cache *fileMetaCache) {
	t.Helper()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, budget := range []*fileMetaBudget{&cache.listed, &cache.pinned} {
		if budget.order.Len() > budget.maxEntries || budget.bytes > budget.maxBytes || budget.bytes < 0 {
			t.Fatalf("缓存超限：entries=%d bytes=%d", budget.order.Len(), budget.bytes)
		}
		var cost int64
		for element := budget.order.Front(); element != nil; element = element.Next() {
			entry := element.Value.(*fileMetaEntry)
			cost += entry.cost
			if cache.byKey[entry.key] != entry || cache.budget(entry.pinned) != budget {
				t.Fatal("LRU 与文件索引不一致")
			}
		}
		if cost != budget.bytes {
			t.Fatalf("缓存计量不一致：%d != %d", cost, budget.bytes)
		}
	}
	if len(cache.byKey) != cache.listed.order.Len()+cache.pinned.order.Len() || len(cache.byDir) > len(cache.byKey) {
		t.Fatal("缓存索引未随淘汰释放")
	}
	for dir, keys := range cache.byDir {
		if len(keys) == 0 {
			t.Fatal("保留了空目录索引")
		}
		for key := range keys {
			if entry := cache.byKey[key]; entry == nil || entry.dir != dir {
				t.Fatal("目录索引保留了失效条目")
			}
		}
	}
}

func TestFileMetaCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newFileMetaCache(2, 1<<20, 1, 1<<20)
	cache.rememberListed("user", "drive", "first", []model.File{{FileID: "a"}, {FileID: "b"}})
	cache.Get("user", "drive", "a")
	cache.rememberListed("user", "drive", "second", []model.File{{FileID: "c"}})
	if _, ok := cache.Get("user", "drive", "b"); ok {
		t.Fatal("未淘汰最久未使用的文件")
	}
	if _, ok := cache.Get("user", "drive", "a"); !ok {
		t.Fatal("淘汰了最近读取的文件")
	}
	for i := range 1000 {
		cache.rememberListed("user", "drive", fmt.Sprint(i), []model.File{{FileID: fmt.Sprint(i)}})
		cache.rememberListed("user", "drive", "empty", nil)
	}
	assertFileMetaBounds(t, cache)
}

func TestFileMetaCacheProtectsPinnedMetadataFromBrowsing(t *testing.T) {
	cache := newFileMetaCache(1, 1<<20, 2, 1<<20)
	want := model.File{FileID: "selected", ParentFileID: "root", Name: "影片", Path: "/root/影片", UserMeta: "下载所需元数据"}
	cache.rememberFile("user", "drive", want)
	for i := range 100 {
		cache.rememberListed("user", "drive", fmt.Sprint(i), []model.File{{FileID: fmt.Sprint(i)}})
	}
	if got, ok := cache.Get("user", "drive", want.FileID); !ok || got != want {
		t.Fatalf("目录浏览淘汰了点击快照：%+v", got)
	}
	cache.rememberFile("user", "drive", model.File{FileID: "next"})
	cache.Get("user", "drive", want.FileID)
	cache.rememberFile("user", "drive", model.File{FileID: "third"})
	if _, ok := cache.Get("user", "drive", "next"); ok {
		t.Fatal("点击快照未按独立容量淘汰")
	}
	assertFileMetaBounds(t, cache)
}

func TestFileMetaCacheLimitsBytesAndRejectsOversizedEntry(t *testing.T) {
	cache := newFileMetaCache(100, 4096, 100, 4096)
	for i := range 100 {
		file := model.File{FileID: fmt.Sprint(i), UserMeta: strings.Repeat("x", 1000)}
		cache.rememberListed("user", "", fmt.Sprint(i), []model.File{file})
		cache.rememberFile("other", "", file)
	}
	cache.rememberFile("user", "", model.File{FileID: "huge", Description: strings.Repeat("x", 8192)})
	if _, ok := cache.Get("user", "", "huge"); ok {
		t.Fatal("接受了超过单组容量的条目")
	}
	assertFileMetaBounds(t, cache)
}

func TestFileMetaCacheRefreshAndInvalidationStayScoped(t *testing.T) {
	cache := newFileMetaCache(20, 1<<20, 20, 1<<20)
	for _, user := range []string{"first", "second"} {
		cache.rememberListed(user, "", "root", []model.File{{FileID: "same", Name: user}})
	}
	cache.rememberFile("first", "", model.File{FileID: "same", ParentFileID: "root", Name: "旧快照"})
	cache.rememberListed("first", "", "root", []model.File{{FileID: "same", Name: "新名称"}})
	if got, _ := cache.Get("first", "", "same"); got.Name != "新名称" {
		t.Fatal("刷新目录未更新点击快照")
	}
	cache.invalidate("first", "", "root")
	if _, ok := cache.Get("first", "", "same"); ok {
		t.Fatal("空空间 ID 的目录未清理")
	}
	if got, ok := cache.Get("second", "", "same"); !ok || got.Name != "second" {
		t.Fatal("清理影响了其他账号")
	}
	cache.rememberFile("first", "", model.File{FileID: "no-parent"})
	cache.invalidate("first", "", "")
	if _, ok := cache.Get("first", "", "no-parent"); ok {
		t.Fatal("全空间清理遗漏了无父目录的快照")
	}
	assertFileMetaBounds(t, cache)
}

func TestFileMetaCacheReplacesDirectoryWithoutDeletingMovedEntry(t *testing.T) {
	cache := newFileMetaCache(10, 1<<20, 10, 1<<20)
	cache.rememberListed("user", "drive", "old", []model.File{{FileID: "moved"}, {FileID: "deleted"}})
	cache.rememberListed("user", "drive", "new", []model.File{{FileID: "moved", ParentFileID: "new"}})
	cache.rememberListed("user", "drive", "old", nil)
	if _, ok := cache.Get("user", "drive", "deleted"); ok {
		t.Fatal("目录刷新保留了已消失的条目")
	}
	if got, ok := cache.Get("user", "drive", "moved"); !ok || got.ParentFileID != "new" {
		t.Fatal("旧目录刷新删除了新目录的文件")
	}
	assertFileMetaBounds(t, cache)
	cache.clear()
	if len(cache.byKey) != 0 || len(cache.byDir) != 0 {
		t.Fatal("清理未释放缓存索引")
	}
	assertFileMetaBounds(t, cache)
}

func TestFileMetaCacheConcurrentAccess(t *testing.T) {
	cache := newFileMetaCache(32, 64<<10, 8, 16<<10)
	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			user := fmt.Sprint(worker)
			for i := range 100 {
				file := model.File{FileID: fmt.Sprint(i), ParentFileID: "root"}
				cache.rememberListed(user, "", "root", []model.File{file})
				cache.rememberFile(user, "", file)
				cache.Get(user, "", file.FileID)
				if i%10 == 0 {
					cache.invalidate(user, "", "root")
				}
			}
		})
	}
	workers.Wait()
	assertFileMetaBounds(t, cache)
}
