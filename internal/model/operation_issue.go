package model

type OperationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action"`
	RetryAt int64  `json:"retryAt,omitempty"`
}
