package model

// File is the unified list/detail file model surfaced to the frontend.
// Field JSON keys intentionally mirror the legacy Electron model so the
// UI layer can reuse shapes without comment.
type File struct {
	DriveID       string `json:"drive_id"`
	FileID        string `json:"file_id"`
	ParentFileID  string `json:"parent_file_id"`
	Name          string `json:"name"`
	NameSearch    string `json:"namesearch"`
	Path          string `json:"path,omitempty"`
	Ext           string `json:"ext"`
	MimeType      string `json:"mime_type"`
	MimeExtension string `json:"mime_extension"`
	Category      string `json:"category"`
	Icon          string `json:"icon"`
	FileCount     int64  `json:"file_count,omitempty"`
	Size          int64  `json:"size"`
	SizeStr       string `json:"sizeStr"`
	Time          int64  `json:"time"`
	TimeStr       string `json:"timeStr"`
	Starred       bool   `json:"starred"`
	IsDir         bool   `json:"isDir"`
	Thumbnail     string `json:"thumbnail"`
	PunishFlag    int    `json:"punish_flag,omitempty"`
	FromShareID   string `json:"from_share_id,omitempty"`
	Description   string `json:"description"`
	ContentHash   string `json:"content_hash,omitempty"`
	ContentHashName string `json:"content_hash_name,omitempty"`
	CRC64Hash     string `json:"crc64_hash,omitempty"`
	AlbumID       string `json:"album_id,omitempty"`
	CompilationID string `json:"compilation_id,omitempty"`
	DownloadURL   string `json:"download_url,omitempty"`

	MediaWidth      int    `json:"media_width,omitempty"`
	MediaHeight     int    `json:"media_height,omitempty"`
	MediaDuration   string `json:"media_duration,omitempty"`
	MediaPlayCursor string `json:"media_play_cursor,omitempty"`
	MediaTime       string `json:"media_time,omitempty"`
	UserMeta        string `json:"user_meta,omitempty"`
}

// SizeString renders a human readable size, e.g. 1.5 MB.
func (f *File) SizeString() string {
	return FormatBytes(f.Size)
}

// VideoQuality is one playable quality variant of a video file.
type VideoQuality struct {
	HTML       string            `json:"html"`
	Quality    string            `json:"quality"`
	Height     int               `json:"height"`
	Width      int               `json:"width"`
	Label      string            `json:"label"`
	Value      string            `json:"value"`
	URL        string            `json:"url"`
	Type       string            `json:"type,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	ForceProxy bool              `json:"forceProxy,omitempty"`
}

// Subtitle is an external subtitle stream URL.
type Subtitle struct {
	Language string            `json:"language"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// VideoPreview holds the resolvable playback sources for a file.
type VideoPreview struct {
	DriveID    string            `json:"drive_id"`
	FileID     string            `json:"file_id"`
	Size       int64             `json:"size"`
	Duration   int64             `json:"duration"`
	ExpireTime int64             `json:"expire_time"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	Headers    map[string]string `json:"headers,omitempty"`
	NoOrigin   bool              `json:"no_origin,omitempty"`
	Qualities  []VideoQuality    `json:"qualities,omitempty"`
	Subtitles  []Subtitle        `json:"subtitles,omitempty"`

	// URL is the local proxy URL the frontend <video> element should load.
	URL            string `json:"url,omitempty"`
	CurrentQuality string `json:"current_quality,omitempty"`
}

// DownloadURL holds a resolvable direct download source for a file.
type DownloadURL struct {
	DriveID        string            `json:"drive_id"`
	FileID         string            `json:"file_id"`
	ExpireTime     int64             `json:"expire_time"`
	URL            string            `json:"url"`
	Size           int64             `json:"size"`
	Headers        map[string]string `json:"headers,omitempty"`
	DownloadMode   string            `json:"downloadMode,omitempty"` // redirect | proxy
	ForceLocalProxy bool             `json:"forceLocalProxy,omitempty"`
	Concurrency    int               `json:"concurrency,omitempty"`
	ChunkSize      int64             `json:"chunkSize,omitempty"`
}

// FolderSize aggregates a folder subtree size/count.
type FolderSize struct {
	Size        int64 `json:"size"`
	FolderCount int64 `json:"folder_count"`
	FileCount   int64 `json:"file_count"`
	ReachLimit  bool  `json:"reach_limit,omitempty"`
}

// Quota models a provider account capacity snapshot.
type Quota struct {
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	SizeStr     string `json:"sizeStr"`
	Used        int64  `json:"used"`
	UsedStr     string `json:"usedStr"`
	Expired     string `json:"expired,omitempty"`
	Description string `json:"description,omitempty"`
}