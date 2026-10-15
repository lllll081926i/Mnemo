package model

type DeletionRecord struct {
	UserID       string `json:"userId"`
	DriveID      string `json:"driveId"`
	FileID       string `json:"fileId"`
	Name         string `json:"name"`
	ParentID     string `json:"parentId"`
	IsDir        bool   `json:"isDir"`
	Size         int64  `json:"size"`
	ProviderType string `json:"providerType"`
	IDKind       string `json:"idKind"`
	Status       string `json:"status"`
	DeletedAt    int64  `json:"deletedAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type CapabilityState struct {
	State      string   `json:"state"`
	Reason     string   `json:"reason"`
	Requires   []string `json:"requires,omitempty"`
	VerifiedAt int64    `json:"verifiedAt,omitempty"`
}
