package store

import (
	"os"
	"time"
)

const settingsFile = "settings.json"

// Settings is the persisted application settings document.
type Settings struct {
	Theme       string `json:"theme"`
	DefaultTab  string `json:"defaultTab"`
	Proxy       string `json:"proxy,omitempty"`
	DownloadDir string `json:"downloadDir,omitempty"`

	MaxConcurrentDownloads int   `json:"maxConcurrentDownloads"`
	MaxDownloadSpeed       int64 `json:"maxDownloadSpeed"` // bytes/s, 0 = unlimited
	MaxUploadSpeed         int64 `json:"maxUploadSpeed"`

	AutoUpdate  bool `json:"autoUpdate"`
	ConfirmUpdate bool `json:"confirmUpdate"`
	PlaybackResume bool `json:"playbackResume"`
	KeepTasks   bool `json:"keepTasks"`
}

// DefaultSettings returns sane defaults.
func DefaultSettings() Settings {
	return Settings{
		Theme:                   "dark",
		DefaultTab:              "pan",
		MaxConcurrentDownloads:  3,
		MaxDownloadSpeed:        0,
		MaxUploadSpeed:          0,
		AutoUpdate:              true,
		ConfirmUpdate:           true,
		PlaybackResume:          true,
		KeepTasks:               true,
	}
}

// GetSettings loads settings, falling back to defaults.
func (s *Store) GetSettings() (Settings, error) {
	st := DefaultSettings()
	err := s.readJSON(settingsFile, &st)
	if err != nil && !os.IsNotExist(err) {
		return st, err
	}
	return st, nil
}

// SetSettings persists settings.
func (s *Store) SetSettings(st Settings) error {
	return s.writeJSON(settingsFile, st)
}

// LocalTag is a user-defined colored label attached to a file path locally.
type LocalTag struct {
	UserID   string `json:"user_id"`
	DriveID  string `json:"drive_id"`
	FileID   string `json:"file_id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	TagID    string `json:"tag_id"`
	Created  int64  `json:"created"`
}

// TagDef is the tag palette definition (name + color).
type TagDef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

const tagsFile = "tags.json"

type tagsDoc struct {
	Defs []TagDef `json:"defs"`
	Marks []LocalTag `json:"marks"`
}

// ListTags returns the tag palette.
func (s *Store) ListTags() ([]TagDef, error) {
	doc, err := s.loadTags()
	if err != nil {
		return nil, err
	}
	return doc.Defs, nil
}

// SaveTags replaces the tag palette.
func (s *Store) SaveTags(defs []TagDef) error {
	doc, err := s.loadTags()
	if err != nil {
		return err
	}
	doc.Defs = defs
	return s.writeJSON(tagsFile, doc)
}

// ListLocalTags returns local marks for an account.
func (s *Store) ListLocalTags(userID, driveID string) ([]LocalTag, error) {
	doc, err := s.loadTags()
	if err != nil {
		return nil, err
	}
	var out []LocalTag
	for _, m := range doc.Marks {
		if (userID == "" || m.UserID == userID) && (driveID == "" || m.DriveID == driveID) {
			out = append(out, m)
		}
	}
	return out, nil
}

// UpsertLocalTag adds or updates a local mark.
func (s *Store) UpsertLocalTag(t LocalTag) error {
	doc, err := s.loadTags()
	if err != nil {
		return err
	}
	replaced := false
	for i, m := range doc.Marks {
		if m.UserID == t.UserID && m.DriveID == t.DriveID && m.FileID == t.FileID {
			doc.Marks[i] = t
			replaced = true
			break
		}
	}
	if !replaced {
		if t.Created == 0 {
			t.Created = time.Now().Unix()
		}
		doc.Marks = append(doc.Marks, t)
	}
	return s.writeJSON(tagsFile, doc)
}

// DeleteLocalTag removes a local mark.
func (s *Store) DeleteLocalTag(userID, driveID, fileID string) error {
	doc, err := s.loadTags()
	if err != nil {
		return err
	}
	out := doc.Marks[:0]
	for _, m := range doc.Marks {
		if m.UserID == userID && m.DriveID == driveID && m.FileID == fileID {
			continue
		}
		out = append(out, m)
	}
	doc.Marks = out
	return s.writeJSON(tagsFile, doc)
}

// CleanupLocalTags removes marks referencing deleted files.
func (s *Store) CleanupLocalTags(userID, driveID string, alive map[string]bool) error {
	doc, err := s.loadTags()
	if err != nil {
		return err
	}
	out := doc.Marks[:0]
	for _, m := range doc.Marks {
		if m.UserID == userID && m.DriveID == driveID {
			if !alive[m.FileID] {
				continue
			}
		}
		out = append(out, m)
	}
	doc.Marks = out
	return s.writeJSON(tagsFile, doc)
}

func (s *Store) loadTags() (tagsDoc, error) {
	var doc tagsDoc
	err := s.readJSON(tagsFile, &doc)
	if err != nil && !os.IsNotExist(err) {
		return doc, err
	}
	return doc, nil
}

// Favorites mirror local favorite marks (decorating provider favorites).
type Favorite struct {
	UserID  string `json:"user_id"`
	DriveID string `json:"drive_id"`
	FileID  string `json:"file_id"`
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Added   int64  `json:"added"`
}

const favoritesFile = "favorites.json"

// ListFavorites returns favorites of an account.
func (s *Store) ListFavorites(userID, driveID string) ([]Favorite, error) {
	var list []Favorite
	err := s.readJSON(favoritesFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var out []Favorite
	for _, f := range list {
		if (userID == "" || f.UserID == userID) && (driveID == "" || f.DriveID == driveID) {
			out = append(out, f)
		}
	}
	return out, nil
}

// AddFavorite records a favorite.
func (s *Store) AddFavorite(f Favorite) error {
	var list []Favorite
	err := s.readJSON(favoritesFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for i, x := range list {
		if x.UserID == f.UserID && x.DriveID == f.DriveID && x.FileID == f.FileID {
			list[i] = f
			return s.writeJSON(favoritesFile, list)
		}
	}
	if f.Added == 0 {
		f.Added = time.Now().Unix()
	}
	list = append(list, f)
	return s.writeJSON(favoritesFile, list)
}

// RemoveFavorite removes a favorite.
func (s *Store) RemoveFavorite(userID, driveID, fileID string) error {
	var list []Favorite
	err := s.readJSON(favoritesFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out := list[:0]
	for _, x := range list {
		if x.UserID == userID && x.DriveID == driveID && x.FileID == fileID {
			continue
		}
		out = append(out, x)
	}
	return s.writeJSON(favoritesFile, out)
}

// LocalPlayCursor stores per-file playback progress for resume.
type PlayCursor struct {
	UserID  string `json:"user_id"`
	DriveID string `json:"drive_id"`
	FileID  string `json:"file_id"`
	Seconds float64 `json:"seconds"`
	Updated int64  `json:"updated"`
}

const cursorFile = "playcursor.json"

// GetPlayCursor returns the saved cursor.
func (s *Store) GetPlayCursor(userID, driveID, fileID string) (float64, error) {
	var list []PlayCursor
	err := s.readJSON(cursorFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for _, c := range list {
		if c.UserID == userID && c.DriveID == driveID && c.FileID == fileID {
			return c.Seconds, nil
		}
	}
	return 0, nil
}

// SavePlayCursor persists playback progress.
func (s *Store) SavePlayCursor(c PlayCursor) error {
	var list []PlayCursor
	err := s.readJSON(cursorFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for i, x := range list {
		if x.UserID == c.UserID && x.DriveID == c.DriveID && x.FileID == c.FileID {
			list[i] = c
			return s.writeJSON(cursorFile, list)
		}
	}
	if c.Updated == 0 {
		c.Updated = time.Now().Unix()
	}
	list = append(list, c)
	return s.writeJSON(cursorFile, list)
}