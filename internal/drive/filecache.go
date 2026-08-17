package drive

import (
	"strings"
	"sync"

	"mnemo-go/internal/model"
)

// fileMetaCache is a small in-memory index of recently listed files keyed by
// account + driveID + fileID. Account scope is required because several
// providers can expose the same or an empty drive ID for different accounts.
type fileMetaCache struct {
	mu    sync.RWMutex
	byKey map[string]model.File
	byDir map[string]map[string]struct{} // account:driveID:dirID -> fileIDs
}

var fileCache = &fileMetaCache{byKey: map[string]model.File{}, byDir: map[string]map[string]struct{}{}}

const cacheKeySeparator = "\x00"

func fileKey(userID, driveID, fileID string) string {
	return userID + cacheKeySeparator + driveID + cacheKeySeparator + fileID
}
func dirKey(userID, driveID, dirID string) string {
	return userID + cacheKeySeparator + driveID + cacheKeySeparator + dirID
}

// Get returns a cached file (copies to avoid data races).
func (c *fileMetaCache) Get(userID, driveID, fileID string) (model.File, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	f, ok := c.byKey[fileKey(userID, driveID, fileID)]
	return f, ok
}

// remember indexes a listed file and registers it under its dir.
func remember(userID, driveID, fileID string, f *model.File) {
	if f == nil || fileID == "" {
		return
	}
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	fileCache.byKey[fileKey(userID, driveID, fileID)] = *f
}

// RememberFile pins one file snapshot for operations triggered directly from
// a list row, such as download or preview. The drive ID keeps accounts with
// the same provider from sharing metadata.
func RememberFile(userID, driveID string, f model.File) {
	remember(userID, driveID, f.FileID, &f)
}

// CachedFile returns a copy of one cached file snapshot.
func CachedFile(userID, driveID, fileID string) (model.File, bool) {
	return fileCache.Get(userID, driveID, fileID)
}

// RememberListedFiles indexes a full listing (replacing the dir bucket).
func RememberListedFiles(userID, driveID, dirID string, files []model.File) {
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	k := dirKey(userID, driveID, dirID)
	if old, ok := fileCache.byDir[k]; ok {
		for id := range old {
			delete(fileCache.byKey, fileKey(userID, driveID, id))
		}
	}
	ids := make(map[string]struct{}, len(files))
	for i := range files {
		f := files[i]
		fileCache.byKey[fileKey(userID, driveID, f.FileID)] = f
		ids[f.FileID] = struct{}{}
	}
	fileCache.byDir[k] = ids
}

// Invalidate removes cached entries for a drive/dir (after mutations).
func InvalidateDriveDirCache(userID, driveID, dirID string) {
	if driveID == "" {
		return
	}
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	if dirID != "" {
		k := dirKey(userID, driveID, dirID)
		if ids, ok := fileCache.byDir[k]; ok {
			for id := range ids {
				delete(fileCache.byKey, fileKey(userID, driveID, id))
			}
		}
		delete(fileCache.byDir, k)
		return
	}
	prefix := userID + cacheKeySeparator + driveID + cacheKeySeparator
	for k, ids := range fileCache.byDir {
		if strings.HasPrefix(k, prefix) {
			for id := range ids {
				delete(fileCache.byKey, fileKey(userID, driveID, id))
			}
			delete(fileCache.byDir, k)
		}
	}
}

// ClearFileMetaCache drops all in-memory metadata after the user clears cache.
func ClearFileMetaCache() {
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	fileCache.byKey = map[string]model.File{}
	fileCache.byDir = map[string]map[string]struct{}{}
}

// Lookup returns the cached file kind (isDir) if known.
func Lookup(userID, driveID, fileID string) (isDir bool, ok bool) {
	f, ok := fileCache.Get(userID, driveID, fileID)
	return f.IsDir, ok
}
