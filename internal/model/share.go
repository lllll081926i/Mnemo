package model

// ShareItem is the unified share record surfaced to the frontend.
type ShareItem struct {
	AccountID       string   `json:"account_id,omitempty"`
	AccountName     string   `json:"account_name,omitempty"`
	AccountProvider string   `json:"account_provider,omitempty"`
	ShareKey        string   `json:"share_key,omitempty"`
	CreatedAt       string   `json:"created_at"`
	Creator         string   `json:"creator,omitempty"`
	Description     string   `json:"description,omitempty"`
	DisplayName     string   `json:"display_name,omitempty"`
	DisplayLabel    string   `json:"display_label,omitempty"`
	DownloadCount   int64    `json:"download_count,omitempty"`
	DriveID         string   `json:"drive_id,omitempty"`
	Expiration      string   `json:"expiration,omitempty"`
	Expired         bool     `json:"expired,omitempty"`
	FileID          string   `json:"file_id,omitempty"`
	FileIDList      []string `json:"file_id_list,omitempty"`
	Icon            string   `json:"icon,omitempty"`
	PreviewCount    int64    `json:"preview_count,omitempty"`
	SaveCount       int64    `json:"save_count,omitempty"`
	ShareID         string   `json:"share_id"`
	ShareMsg        string   `json:"share_msg,omitempty"`
	FullShareMsg    string   `json:"full_share_msg,omitempty"`
	ShareName       string   `json:"share_name,omitempty"`
	SharePolicy     string   `json:"share_policy,omitempty"`
	SharePwd        string   `json:"share_pwd,omitempty"`
	ShareURL        string   `json:"share_url,omitempty"`
	Status          string   `json:"status,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	IsShareSaved    bool     `json:"is_share_saved,omitempty"`
}

// ShareHistoryEntry is a locally persisted share record (searchable history).
type ShareHistoryEntry struct {
	ShareID   string `json:"share_id"`
	AccountID string `json:"account_id"`
	DriveID   string `json:"drive_id"`
	FileID    string `json:"file_id"`
	ShareURL  string `json:"share_url"`
	SharePwd  string `json:"share_pwd,omitempty"`
	ShareName string `json:"share_name"`
	CreatedAt int64  `json:"created_at"`
	Provider  string `json:"provider"`
}

// ImportedShareResult is the outcome of importing a share link.
type ImportedShareResult struct {
	Provider string `json:"provider"`
	ShareID  string `json:"share_id"`
	Name     string `json:"name,omitempty"`
	Files    []File `json:"files,omitempty"`
	Message  string `json:"message,omitempty"`
}
