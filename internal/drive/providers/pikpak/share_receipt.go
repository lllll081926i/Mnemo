package pikpak

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

type shareTaskRef struct {
	ID     string `json:"id"`
	TaskID string `json:"task_id"`
}

type shareTaskReceipt struct {
	shareTaskRef
	Task  *shareTaskRef  `json:"task"`
	Tasks []shareTaskRef `json:"tasks"`
}

func (receipt shareTaskReceipt) taskIDs() []string {
	result := []string{}
	seen := map[string]bool{}
	references := append([]shareTaskRef{{TaskID: receipt.TaskID}}, receipt.Tasks...)
	if receipt.Task != nil {
		references = append(references, *receipt.Task)
	}
	for _, reference := range references {
		identity := strings.TrimSpace(firstNonEmpty(reference.TaskID, reference.ID))
		if identity != "" && !seen[identity] {
			seen[identity] = true
			result = append(result, identity)
		}
	}
	return result
}

func (d *Driver) ValidateShareSession(session *drive.ShareImportSession) error {
	if session == nil || session.Provider != providerID || strings.TrimSpace(session.ShareID) == "" || strings.TrimSpace(session.PassCodeToken) == "" {
		return errors.New("pikpak: 分享会话无效，请重新解析链接")
	}
	return nil
}

func (d *Driver) VerifyShareReceipt(ctx context.Context, operation drive.Context, items []model.ShareImportItem) ([]model.ShareImportItem, error) {
	client, err := clientOf(operation)
	if err != nil {
		return nil, err
	}
	states := map[string]string{}
	result := append([]model.ShareImportItem(nil), items...)
	for index, item := range items {
		if item.Status == "completed" || item.Status == "failed" {
			continue
		}
		identities := append([]string(nil), item.TaskIDs...)
		if item.TaskID != "" {
			identities = append(identities, item.TaskID)
		}
		if len(identities) == 0 {
			continue
		}
		status := "completed"
		for _, identity := range identities {
			state, known := states[identity]
			if !known {
				task, err := client.findTask(ctx, identity)
				if err != nil {
					return nil, err
				}
				if task == nil || task.TaskID != identity {
					return nil, fmt.Errorf("pikpak: 转存任务回执身份不匹配")
				}
				switch strings.ToLower(firstNonEmpty(task.Phase, task.Status)) {
				case "phase_type_complete", "complete", "completed":
					state = "completed"
				case "phase_type_error", "error", "failed":
					state = "failed"
				default:
					state = "submitted"
				}
				states[identity] = state
			}
			if state == "failed" {
				status = "failed"
			} else if state != "completed" && status != "failed" {
				status = "submitted"
			}
		}
		result[index].Status = status
	}
	return result, nil
}
