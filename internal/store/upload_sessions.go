package store

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

// uploadSessionFile is the on-disk directory for per-key upload session JSON.
const uploadSessionDir = "upload_sessions"

// UploadSessionRecord stores the set of uploaded part numbers for a single
// resumable upload. The key is a stable hash of userID:driveID:parentID:name:size.
type UploadSessionRecord struct {
	Key                 string `json:"key"`
	SessionID           string `json:"sessionId,omitempty"`
	UploadedPartNumbers []int  `json:"uploadedPartNumbers"`
}

// uploadSessionStore is a lightweight per-file JSON store. Each session key gets
// its own file under <storeDir>/upload_sessions/<key>.json so concurrent uploads
// don't contend on a single mutex for unrelated keys.
var (
	usMu       sync.Mutex
	usStoreDir string
)

// InitUploadSessions wires the store directory for upload session persistence.
// Called once at startup by the app layer (via drive.SetUploadSessionStore).
func InitUploadSessions(dir string) {
	usMu.Lock()
	defer usMu.Unlock()
	usStoreDir = filepath.Join(dir, uploadSessionDir)
	_ = os.MkdirAll(usStoreDir, 0o755)
}

// UploadSessionKey computes a stable hash key from the tuple
// userID:driveID:parentID:name:size.
func UploadSessionKey(userID, driveID, parentID, name string, size int64) string {
	raw := userID + ":" + driveID + ":" + parentID + ":" + name + ":" + strconv.FormatInt(size, 10)
	h := sha1.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

// SaveUploadSession persists the uploaded part numbers for a session key.
func SaveUploadSession(key string, partNumbers []int) error {
	return SaveUploadSessionState(key, "", partNumbers)
}

// SaveUploadSessionState persists a provider session id with completed parts.
// The id prevents parts from one remote session being applied to a new one.
func SaveUploadSessionState(key, sessionID string, partNumbers []int) error {
	usMu.Lock()
	dir := usStoreDir
	usMu.Unlock()
	if dir == "" {
		return nil
	}
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	rec := UploadSessionRecord{
		Key:                 key,
		SessionID:           sessionID,
		UploadedPartNumbers: partNumbers,
	}
	path := filepath.Join(dir, key+".json")
	tmp := path + ".tmp"
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadUploadSession returns the persisted uploaded part numbers for a key,
// or nil if no session exists.
func LoadUploadSession(key string) []int {
	_, parts := LoadUploadSessionState(key)
	return parts
}

// LoadUploadSessionState returns the provider session id and completed parts.
func LoadUploadSessionState(key string) (string, []int) {
	usMu.Lock()
	dir := usStoreDir
	usMu.Unlock()
	if dir == "" {
		return "", nil
	}
	b, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return "", nil
	}
	var rec UploadSessionRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", nil
	}
	return rec.SessionID, rec.UploadedPartNumbers
}

// ClearUploadSession removes the persisted session for a key.
func ClearUploadSession(key string) {
	usMu.Lock()
	dir := usStoreDir
	usMu.Unlock()
	if dir == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, key+".json"))
}

// SortedUniqueParts deduplicates and sorts part numbers for stable persistence.
func SortedUniqueParts(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
