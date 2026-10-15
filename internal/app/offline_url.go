package app

import "mnemo-go/internal/model"

func safeOfflineDisplayURL(raw string) string {
	return model.SafeOfflineDisplayURL(raw)
}
