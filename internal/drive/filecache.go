package drive

import (
	"container/list"
	"strings"
	"sync"

	"mnemo-go/internal/model"
)

// 两组 LRU 分别限制目录浏览和点击快照，浏览不会挤掉刚选中的文件。
type fileMetaCache struct {
	mu          sync.Mutex
	byKey       map[string]*fileMetaEntry
	byDir       map[string]map[string]struct{}
	listed      fileMetaBudget
	pinned      fileMetaBudget
	listVersion uint64
}

type fileMetaBudget struct {
	order      list.List
	bytes      int64
	maxEntries int
	maxBytes   int64
}

type fileMetaEntry struct {
	file       model.File
	key, dir   string
	cost       int64
	pinned     bool
	generation uint64
	element    *list.Element
}

var fileCache = newFileMetaCache(20000, 64<<20, 1024, 8<<20)

func newFileMetaCache(listedEntries int, listedBytes int64, pinnedEntries int, pinnedBytes int64) *fileMetaCache {
	return &fileMetaCache{
		byKey:  make(map[string]*fileMetaEntry),
		byDir:  make(map[string]map[string]struct{}),
		listed: fileMetaBudget{maxEntries: listedEntries, maxBytes: listedBytes},
		pinned: fileMetaBudget{maxEntries: pinnedEntries, maxBytes: pinnedBytes},
	}
}

const cacheKeySeparator = "\x00"

func fileKey(userID, driveID, fileID string) string {
	return userID + cacheKeySeparator + driveID + cacheKeySeparator + fileID
}
func dirKey(userID, driveID, dirID string) string {
	return userID + cacheKeySeparator + driveID + cacheKeySeparator + dirID
}

// Get 返回副本并更新最近使用顺序。
func (c *fileMetaCache) Get(userID, driveID, fileID string) (model.File, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.byKey[fileKey(userID, driveID, fileID)]
	if !ok {
		return model.File{}, false
	}
	c.budget(entry.pinned).order.MoveToFront(entry.element)
	return entry.file, true
}

func (c *fileMetaCache) budget(pinned bool) *fileMetaBudget {
	if pinned {
		return &c.pinned
	}
	return &c.listed
}

// 估算文件字段和索引占用；条目数同时限制 map、链表与目录索引开销。
func fileMetaCost(key, dir string, f model.File) int64 {
	return 1024 + int64(len(key)+len(dir)+len(f.DriveID)+len(f.FileID)+len(f.ParentFileID)+
		len(f.Name)+len(f.NameSearch)+len(f.Path)+len(f.Ext)+len(f.MimeType)+len(f.MimeExtension)+
		len(f.Category)+len(f.Icon)+len(f.SizeStr)+len(f.TimeStr)+len(f.Thumbnail)+len(f.FromShareID)+
		len(f.Description)+len(f.ContentHash)+len(f.ContentHashName)+len(f.CRC64Hash)+len(f.AlbumID)+
		len(f.CompilationID)+len(f.DownloadURL)+len(f.MediaDuration)+len(f.MediaPlayCursor)+
		len(f.MediaTime)+len(f.UserMeta))
}

func (c *fileMetaCache) remove(entry *fileMetaEntry) {
	delete(c.byKey, entry.key)
	budget := c.budget(entry.pinned)
	budget.order.Remove(entry.element)
	budget.bytes -= entry.cost
	if ids := c.byDir[entry.dir]; ids != nil {
		delete(ids, entry.key)
		if len(ids) == 0 {
			delete(c.byDir, entry.dir)
		}
	}
}

// 调用者持锁。目录索引只引用仍在缓存中的条目，不保留空目录。
func (c *fileMetaCache) put(key, dir string, f model.File, pinned bool, generation uint64) {
	if f.FileID == "" {
		return
	}
	if previous := c.byKey[key]; previous != nil {
		pinned = pinned || previous.pinned
		if dir == "" {
			dir = previous.dir
		}
		c.remove(previous)
	}
	budget := c.budget(pinned)
	cost := fileMetaCost(key, dir, f)
	if budget.maxEntries <= 0 || cost > budget.maxBytes {
		return
	}
	entry := &fileMetaEntry{file: f, key: key, dir: dir, cost: cost, pinned: pinned, generation: generation}
	entry.element = budget.order.PushFront(entry)
	budget.bytes += cost
	c.byKey[key] = entry
	if dir != "" {
		if c.byDir[dir] == nil {
			c.byDir[dir] = make(map[string]struct{})
		}
		c.byDir[dir][key] = struct{}{}
	}
	for budget.order.Len() > budget.maxEntries || budget.bytes > budget.maxBytes {
		c.remove(budget.order.Back().Value.(*fileMetaEntry))
	}
}

func (c *fileMetaCache) rememberFile(userID, driveID string, f model.File) {
	c.mu.Lock()
	defer c.mu.Unlock()
	dir := ""
	if f.ParentFileID != "" {
		dir = dirKey(userID, driveID, f.ParentFileID)
	}
	c.put(fileKey(userID, driveID, f.FileID), dir, f, true, 0)
}

// 后端详情查询使用普通缓存额度，避免批量查询挤占点击快照。
func remember(userID, driveID, fileID string, f *model.File) {
	if f == nil || fileID == "" {
		return
	}
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	dir := ""
	if f.ParentFileID != "" {
		dir = dirKey(userID, driveID, f.ParentFileID)
	}
	fileCache.put(fileKey(userID, driveID, fileID), dir, *f, false, 0)
}

// RememberFile 保存有独立限额的点击快照，用于下载与预览等文件操作。
func RememberFile(userID, driveID string, f model.File) {
	fileCache.rememberFile(userID, driveID, f)
}

// CachedFile 返回一个账号和空间内的文件副本。
func CachedFile(userID, driveID, fileID string) (model.File, bool) {
	return fileCache.Get(userID, driveID, fileID)
}

func (c *fileMetaCache) rememberListed(userID, driveID, dirID string, files []model.File) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listVersion++
	generation := c.listVersion
	dir := dirKey(userID, driveID, dirID)
	for _, f := range files {
		c.put(fileKey(userID, driveID, f.FileID), dir, f, false, generation)
	}
	for key := range c.byDir[dir] {
		if entry := c.byKey[key]; entry.generation != generation {
			c.remove(entry)
		}
	}
}

// RememberListedFiles 更新完整目录，移除已消失的条目并淘汰最久未使用的元数据。
func RememberListedFiles(userID, driveID, dirID string, files []model.File) {
	fileCache.rememberListed(userID, driveID, dirID, files)
}

func (c *fileMetaCache) invalidate(userID, driveID, dirID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dirID != "" {
		for key := range c.byDir[dirKey(userID, driveID, dirID)] {
			c.remove(c.byKey[key])
		}
		return
	}
	prefix := userID + cacheKeySeparator + driveID + cacheKeySeparator
	for key, entry := range c.byKey {
		if strings.HasPrefix(key, prefix) {
			c.remove(entry)
		}
	}
}

// InvalidateDriveDirCache 清理指定账号与空间的目录，包含点击快照和空空间 ID。
func InvalidateDriveDirCache(userID, driveID, dirID string) {
	fileCache.invalidate(userID, driveID, dirID)
}

func (c *fileMetaCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byKey = make(map[string]*fileMetaEntry)
	c.byDir = make(map[string]map[string]struct{})
	c.listed.order.Init()
	c.listed.bytes = 0
	c.pinned.order.Init()
	c.pinned.bytes = 0
	c.listVersion = 0
}

// ClearFileMetaCache 在用户清理缓存时释放全部索引和点击快照。
func ClearFileMetaCache() {
	fileCache.clear()
}

// Lookup 返回已知的文件类型。
func Lookup(userID, driveID, fileID string) (isDir bool, ok bool) {
	f, ok := fileCache.Get(userID, driveID, fileID)
	return f.IsDir, ok
}
