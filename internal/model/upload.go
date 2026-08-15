package model

// UploadInfo describes the local file being uploaded.
type UploadInfo struct {
	LocalFilePath string `json:"localFilePath"`
	ParentFileID  string `json:"parent_file_id"`
	DriveID       string `json:"drive_id"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	SizeStr       string `json:"sizeStr"`
	Icon          string `json:"icon"`
	IsDir         bool   `json:"isDir"`
	IsMiaoChuan   bool   `json:"isMiaoChuan"` // fingerprint秒传 flag
	SHA1          string `json:"sha1"`
	CRC64         string `json:"crc64"`

	// ConflictPolicy controls behavior when the target path already exists.
	// Supported: "" | "overwrite" (default, legacy) | "refuse" | "rename" | "skip".
	ConflictPolicy string `json:"conflictPolicy,omitempty"`
}

// UploadState is the mutable progress/status of an upload job.
type UploadState struct {
	DownState     string `json:"DownState"` // queued | uploading | completed | failed | stopped
	DownTime      int64  `json:"DownTime"`
	DownSize      int64  `json:"DownSize"`
	DownSpeed     int64  `json:"DownSpeed"`
	DownSpeedStr  string `json:"DownSpeedStr"`
	DownProcess   int    `json:"DownProcess"`
	IsStop        bool   `json:"IsStop"`
	IsDowning     bool   `json:"IsDowning"`
	IsCompleted   bool   `json:"IsCompleted"`
	IsFailed      bool   `json:"IsFailed"`
	FailedCode    int    `json:"failedCode"`
	FailedMessage string `json:"failedMessage"`
	AutoTry       int    `json:"AutoTry"`
	UploadID      string `json:"upload_id"`
	FileID        string `json:"file_id"`
	IsBreakExist  bool   `json:"IsBreakExist"`
}

// UploadingUI is the full upload job view model (mirrors legacy IUploadingUI).
type UploadingUI struct {
	UploadID string      `json:"UploadID"`
	UserID   string      `json:"user_id"`
	Info     UploadInfo  `json:"Info"`
	Upload   UploadState `json:"Upload"`
}

// DownloadTask is a transfer-center download task.
type DownloadTask struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	DriveID    string `json:"drive_id"`
	Provider   string `json:"provider"`
	FileID     string `json:"file_id"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Downloaded int64  `json:"downloaded"`
	Speed      int64  `json:"speed"`
	Progress   int    `json:"progress"`
	Status     string `json:"status"` // queued|downloading|paused|completed|failed|canceled
	LocalPath  string `json:"localPath"`
	URL        string `json:"url,omitempty"`
	Error      string `json:"error,omitempty"`
	Created    int64  `json:"created"`
	Updated    int64  `json:"updated"`
	Concurrency int   `json:"concurrency,omitempty"`
}

// OfflineTask is a provider-side (cloud) offline download task (PikPak).
type OfflineTask struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	DriveID   string `json:"drive_id"`
	TaskID    string `json:"task_id,omitempty"`
	URL       string `json:"url,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Created   int64  `json:"created"`
}

// MigrateJob is one cross-drive migration request.
type MigrateJob struct {
	ID       string   `json:"id"`
	SrcUser  string   `json:"srcUser"`
	SrcDrive string   `json:"srcDrive"`
	FileIDs  []string `json:"fileIDs"`
	DstUser  string   `json:"dstUser"`
	DstDrive string   `json:"dstDrive"`
	DstParent string  `json:"dstParent"`
	Move     bool     `json:"move"`
	// Live progress
	Total     int64 `json:"total"`
	Processed int64 `json:"processed"`
	Failed    int64 `json:"failed"`
	// Byte-level progress (accumulated across files).
	TotalBytes     int64 `json:"totalBytes"`
	ProcessedBytes int64 `json:"processedBytes"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	// Timestamps for persistence.
	CreatedAt int64 `json:"createdAt,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
}