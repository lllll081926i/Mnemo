package app

import (
	"context"
	"errors"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (a *App) SearchFilesCancelable(requestID, userID, driveID, keyword string) ([]model.File, error) {
	if strings.TrimSpace(requestID) == "" || len(requestID) > 256 {
		return nil, errors.New("搜索请求 ID 无效")
	}
	ctx, cancel := context.WithCancel(a.appContext())
	a.searchMu.Lock()
	if a.searchStopping || a.searchRuns[requestID] != nil {
		a.searchMu.Unlock()
		cancel()
		return nil, errors.New("搜索请求重复或应用正在退出")
	}
	if a.searchRuns == nil {
		a.searchRuns = map[string]context.CancelFunc{}
	}
	a.searchRuns[requestID] = cancel
	a.searchMu.Unlock()
	defer func() {
		cancel()
		a.searchMu.Lock()
		delete(a.searchRuns, requestID)
		a.searchMu.Unlock()
	}()
	return drive.SearchDirContext(ctx, userID, driveID, keyword)
}

func (a *App) CancelSearch(requestID string) {
	a.searchMu.Lock()
	cancel := a.searchRuns[requestID]
	a.searchMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) cancelAllSearches() {
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	a.searchStopping = true
	for _, cancel := range a.searchRuns {
		cancel()
	}
}
