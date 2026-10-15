package model

type ShareImportReceipt struct {
	ID           string            `json:"id"`
	AccountID    string            `json:"accountId"`
	DriveID      string            `json:"driveId"`
	ParentID     string            `json:"parentId"`
	Provider     string            `json:"provider"`
	ShareID      string            `json:"shareId"`
	Items        []ShareImportItem `json:"items"`
	CreatedAt    int64             `json:"createdAt"`
	UpdatedAt    int64             `json:"updatedAt"`
	HistorySaved bool              `json:"historySaved"`
	Message      string            `json:"message,omitempty"`
}

type ShareImportItem struct {
	SourceID string   `json:"sourceId"`
	TargetID string   `json:"targetId,omitempty"`
	TaskID   string   `json:"taskId,omitempty"`
	TaskIDs  []string `json:"taskIds,omitempty"`
	Status   string   `json:"status"`
}
