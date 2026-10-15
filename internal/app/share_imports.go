package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

func shareImportIdentity(userID, driveID string, session *drive.ShareImportSession, fileIDs []string, parentID string) string {
	selection := append([]string(nil), fileIDs...)
	sort.Strings(selection)
	parts := []any{userID, driveID, session.Provider, session.ShareID, session.ShareKey, parentID, selection}
	data, _ := json.Marshal(parts)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func shareImportSource(session *drive.ShareImportSession) string {
	if session.ShareID != "" {
		return session.ShareID
	}
	return session.ShareKey
}

func (a *App) ListShareImportReceipts(userID, driveID string) ([]model.ShareImportReceipt, error) {
	storage, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	return storage.ListShareImports(userID, driveID)
}

func (a *App) SaveImportedShareDetailed(userID, driveID string, session *drive.ShareImportSession, fileIDs []string, toParentID string) (*model.ShareImportReceipt, error) {
	if err := drive.ValidateImportedShare(userID, driveID, session, fileIDs); err != nil {
		return nil, err
	}
	if strings.TrimSpace(session.ShareID) == "" && strings.TrimSpace(session.ShareKey) == "" {
		return nil, errors.New("分享会话缺少稳定标识，请重新解析链接")
	}
	root := drive.GetMeta(session.Provider).RootKey
	if toParentID == "" || toParentID == "root" || toParentID == "*" || toParentID == "/" || toParentID == root {
		toParentID = root
	}
	identity := shareImportIdentity(userID, driveID, session, fileIDs, toParentID)
	unlock, err := drive.LockTarget(a.appContext(), userID, "", "share-import", "")
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := drive.ValidateImportedShare(userID, driveID, session, fileIDs); err != nil {
		return nil, err
	}
	storage, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	receipts, err := storage.ListShareImports(userID, driveID)
	if err != nil {
		return nil, err
	}
	for _, existing := range receipts {
		if existing.ID == identity {
			existing.Message = "该批次已有转存回执，未重复提交；请刷新状态或核对目标目录"
			return &existing, nil
		}
		if existing.Provider == session.Provider && existing.ShareID == shareImportSource(session) && existing.ParentID == toParentID {
			selected := map[string]bool{}
			for _, fileID := range fileIDs {
				selected[fileID] = true
			}
			for _, item := range existing.Items {
				if selected[item.SourceID] {
					return nil, errors.New("所选项目在该目录已有转存回执，请先核对原批次；未重复提交")
				}
			}
		}
	}
	receipt := &model.ShareImportReceipt{ID: identity, AccountID: userID, DriveID: driveID, ParentID: toParentID, Provider: session.Provider, ShareID: shareImportSource(session), CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix()}
	for _, fileID := range fileIDs {
		receipt.Items = append(receipt.Items, model.ShareImportItem{SourceID: fileID, Status: "pending"})
	}
	if err := storage.SaveShareImport(*receipt); err != nil {
		return nil, errors.New("无法保存转存检查点，未向网盘提交")
	}
	items, operationErr := drive.SaveImportedShareReceipt(a.appContext(), userID, driveID, session, fileIDs, toParentID)
	receipt.Items = mergeShareImportItems(receipt.Items, items)
	if operationErr != nil {
		receipt.Message = "部分或全部转存结果未确认；请刷新状态并核对目标目录，不要重复提交"
		logging.Warn("share import receipt uncertain", "account_id", redactID(userID), "receipt_id", redactID(identity), "error", operationErr)
	}
	receipt.UpdatedAt = time.Now().Unix()
	receipt.HistorySaved = true
	if err := storage.SaveShareImport(*receipt); err != nil {
		receipt.HistorySaved = false
		receipt.Message = "远端请求已发送，但最终回执保存失败；检查点已保留，不会自动重发，请核对目标目录"
	}
	a.emit("share:imports-changed", map[string]string{"account_id": userID})
	return receipt, nil
}

func mergeShareImportItems(original, updates []model.ShareImportItem) []model.ShareImportItem {
	bySource := map[string]model.ShareImportItem{}
	for _, item := range updates {
		if _, duplicate := bySource[item.SourceID]; duplicate {
			item.Status = "unknown"
		}
		bySource[item.SourceID] = item
	}
	result := make([]model.ShareImportItem, 0, len(original))
	for _, item := range original {
		if item.Status == "completed" || item.Status == "failed" {
			result = append(result, item)
			continue
		}
		update, exists := bySource[item.SourceID]
		if exists {
			switch update.Status {
			case "submitted", "completed", "failed", "unknown":
				item = update
			default:
				item.Status = "unknown"
			}
		} else if item.Status != "completed" && item.Status != "failed" {
			item.Status = "unknown"
		}
		result = append(result, item)
	}
	return result
}

func (a *App) RefreshShareImportReceipt(userID, driveID, receiptID string) (*model.ShareImportReceipt, error) {
	unlock, err := drive.LockTarget(a.appContext(), userID, "", "share-import", "")
	if err != nil {
		return nil, err
	}
	defer unlock()
	storage, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	receipts, err := storage.ListShareImports(userID, driveID)
	if err != nil {
		return nil, err
	}
	for _, receipt := range receipts {
		if receipt.ID != receiptID {
			continue
		}
		items, err := drive.RefreshImportedShareReceipt(a.appContext(), userID, driveID, receipt.Provider, receipt.Items)
		if errors.Is(err, drive.ErrNotImplemented) {
			receipt.Message = "该网盘未提供可核验的转存任务查询，请在目标目录确认结果；不会重新提交"
			return &receipt, nil
		}
		if err != nil {
			return nil, err
		}
		receipt.Items = mergeShareImportItems(receipt.Items, items)
		receipt.Message = ""
		for _, item := range receipt.Items {
			if item.Status != "completed" && item.Status != "failed" && item.TaskID == "" && len(item.TaskIDs) == 0 {
				receipt.Message = "服务端未返回可核验的任务标识，请在目标目录确认结果；不会重新提交"
			}
		}
		receipt.UpdatedAt = time.Now().Unix()
		receipt.HistorySaved = true
		if err := storage.SaveShareImport(receipt); err != nil {
			receipt.HistorySaved = false
			receipt.Message = "状态查询成功，但本机回执保存失败；下次刷新只查询，不重复转存"
		}
		return &receipt, nil
	}
	return nil, errors.New("转存回执不属于当前账号和空间")
}
