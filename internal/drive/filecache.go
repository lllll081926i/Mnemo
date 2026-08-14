package drive

import (
	"sync"

	"mnemo-go/internal/model"
)

// fileMetaCache is a small in-memory index of recently listed files keyed by
// driveID+fileID. It lets batch ops and detail views know isDir/category
// without an extra network round trip. It is best-effort only: misses fall
// back to provider detail calls.
type fileMetaCache struct {
	mu     sync.RWMutex
	byKey  map[string]model.File
	byDir  map[string]map[string]struct{} // driveID:dirID -> fileIDs
}

var fileCache = &fileMetaCache{byKey: map[string]model.File{}, byDir: map[string]map[string]struct{}{}}

func fileKey(driveID, fileID string) string { return driveID + ":" + fileID }
func dirKey(driveID, dirID string) string   { return driveID + ":" + dirID }

// Get returns a cached file (copies to avoid data races).
func (c *fileMetaCache) Get(driveID, fileID string) (model.File, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	f, ok := c.byKey[fileKey(driveID, fileID)]
	return f, ok
}

// remember indexes a listed file and registers it under its dir.
func remember(driveID, fileID string, f *model.File) {
	if f == nil || fileID == "" {
		return
	}
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	fileCache.byKey[fileKey(driveID, fileID)] = *f
}

// RememberListedFiles indexes a full listing (replacing the dir bucket).
func RememberListedFiles(driveID, dirID string, files []model.File) {
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	k := dirKey(driveID, dirID)
	if old, ok := fileCache.byDir[k]; ok {
		for id := range old {
			delete(fileCache.byKey, fileKey(driveID, id))
		}
	}
	ids := make(map[string]struct{}, len(files))
	for i := range files {
		f := files[i]
		fileCache.byKey[fileKey(driveID, f.FileID)] = f
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
		k := dirKey(driveID, dirID)
		if ids, ok := fileCache.byDir[k]; ok {
			for id := range ids {
				delete(fileCache.byKey, fileKey(driveID, id))
			}
		}
		delete(fileCache.byDir, k)
		return
	}
	for k, ids := range fileCache.byDir {
		if len(k) > len(driveID) && k[:len(driveID)] == driveID {
			for id := range ids {
				delete(fileCache.byKey, fileKey(driveID, id))
			}
			delete(fileCache.byDir, k)
		}
	}
}

// Lookup returns the cached file kind (isDir) if known.
func Lookup(driveID, fileID string) (isDir bool, ok bool) {
	f, ok := fileCache.Get(driveID, fileID)
	return f.IsDir, ok
}