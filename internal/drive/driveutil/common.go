// Package driveutil provides shared helpers for provider implementations:
// unified file model construction, category/mime/icon mapping.
package driveutil

import (
	"mime"
	"path"
	"strings"
	"time"

	"mnemo-go/internal/model"
)

// Category kinds aligned with the legacy frontend.
const (
	CatVideo  = "video"
	CatAudio  = "audio"
	CatImage  = "image"
	CatDoc    = "doc"
	CatArchive = "archive"
	CatText   = "text"
	CatOther  = "other"
)

var videoExts = map[string]bool{"mp4": true, "mkv": true, "avi": true, "mov": true, "wmv": true, "flv": true, "webm": true, "m4v": true, "ts": true, "m3u8": true, "rmvb": true, "rm": true, "3gp": true, "mpg": true, "mpeg": true, "m2ts": true}
var audioExts = map[string]bool{"mp3": true, "flac": true, "wav": true, "aac": true, "ogg": true, "m4a": true, "wma": true, "ape": true, "opus": true, "amr": true, "mid": true, "midi": true}
var imageExts = map[string]bool{"jpg": true, "jpeg": true, "png": true, "gif": true, "bmp": true, "webp": true, "svg": true, "ico": true, "tif": true, "tiff": true, "heic": true, "avif": true}
var docExts = map[string]bool{"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true, "ppt": true, "pptx": true, "md": true, "txt": true, "rtf": true, "odt": true, "ods": true, "odp": true, "csv": true}
var archiveExts = map[string]bool{"zip": true, "rar": true, "7z": true, "tar": true, "gz": true, "bz2": true, "xz": true, "zst": true, "iso": true, "cab": true}
var textExts = map[string]bool{"txt": true, "md": true, "log": true, "json": true, "xml": true, "yml": true, "yaml": true, "ini": true, "conf": true, "py": true, "js": true, "ts": true, "go": true, "c": true, "h": true, "cpp": true, "java": true, "rs": true, "sh": true, "bat": true, "html": true, "css": true, "vue": true}

// Ext returns the lowercased extension (without dot) of a name.
func Ext(name string) string {
	e := strings.ToLower(path.Ext(name))
	return strings.TrimPrefix(e, ".")
}

// GuessCategory maps an extension to a file category.
func GuessCategory(name string) string {
	e := Ext(name)
	switch {
	case videoExts[e]:
		return CatVideo
	case audioExts[e]:
		return CatAudio
	case imageExts[e]:
		return CatImage
	case e == "pdf":
		return CatDoc
	case archiveExts[e]:
		return CatArchive
	case textExts[e]:
		return CatText
	case docExts[e]:
		return CatDoc
	default:
		return CatOther
	}
}

// GuessMime maps an extension to a mime type.
func GuessMime(name string) string {
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return strings.SplitN(t, ";", 2)[0]
	}
	return "application/octet-stream"
}

// NewFile builds a unified file model from primitive fields.
func NewFile(driveID, fileID, parentID, name string, isDir bool, size int64, timeUnix int64) model.File {
	category := CatOther
	if !isDir {
		category = GuessCategory(name)
	}
	return model.File{
		DriveID:      driveID,
		FileID:       fileID,
		ParentFileID: parentID,
		Name:         name,
		NameSearch:   strings.ToLower(name),
		Ext:          Ext(name),
		MimeType:     GuessMime(name),
		Category:     category,
		Size:         size,
		SizeStr:      model.FormatBytes(size),
		Time:         timeUnix,
		TimeStr:      formatTime(timeUnix),
		IsDir:        isDir,
	}
}

func formatTime(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}

// JoinPath joins a parent path and a name with forward slashes.
func JoinPath(parent, name string) string {
	if parent == "" || parent == "/" {
		return "/" + name
	}
	return strings.TrimSuffix(parent, "/") + "/" + name
}
