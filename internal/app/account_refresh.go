package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

const (
	accountRefreshTTL          = 45 * time.Minute
	accountRefreshManualGap    = 30 * time.Second
	accountRefreshErrorBackoff = 10 * time.Minute
	accountRefreshRiskBackoff  = time.Hour
)

// retryAfterError is implemented by providers that return a precise server
// cooldown. It deliberately lives in the app layer so providers remain
// independent and no central provider-specific type switch is needed.
type retryAfterError interface {
	RetryAfter() time.Duration
}

// ListAccounts returns all accounts.
func (a *App) ListAccounts() []*model.Account {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListAccounts()
	if err != nil {
		return nil
	}
	for _, acc := range list {
		syncAccountUsage(acc)
	}
	logging.Debug("accounts listed", "count", len(list))
	return list
}

func syncAccountUsage(acc *model.Account) {
	if acc == nil {
		return
	}
	if acc.Usage == nil {
		acc.Usage = &model.Quota{Type: "account", Status: "unknown"}
	}
	if acc.Provider() == model.ProviderYike || acc.Provider() == model.ProviderLanzou {
		acc.Usage.Type = "unlimited"
		acc.Usage.Size = 0
		acc.Usage.SizeStr = ""
		acc.Usage.Used = 0
		acc.Usage.UsedStr = ""
		acc.Usage.Status = "available"
		acc.Usage.Description = "无限空间"
		return
	}
	acc.Usage.Type = "account"
	if acc.Token == nil || acc.Token.TotalSize <= 0 {
		acc.Usage.Size = 0
		acc.Usage.SizeStr = ""
		acc.Usage.Used = 0
		acc.Usage.UsedStr = ""
		if acc.Usage.Status != "rate_limited" && acc.Usage.Status != "error" {
			acc.Usage.Status = "unsupported"
			acc.Usage.Description = "暂无容量信息"
		}
		return
	}
	total := acc.Token.TotalSize
	used := acc.Token.UsedSize
	if used <= 0 && acc.Token.FreeSize >= 0 && acc.Token.FreeSize < total {
		used = total - acc.Token.FreeSize
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}
	acc.Usage.Size = total
	acc.Usage.SizeStr = model.FormatBytes(total)
	acc.Usage.Used = used
	acc.Usage.UsedStr = model.FormatBytes(used)
	if acc.Usage.Status == "" || acc.Usage.Status == "unknown" || acc.Usage.Status == "unsupported" {
		acc.Usage.Status = "available"
		acc.Usage.Description = ""
	}
}

func markQuotaRefreshSuccess(acc *model.Account) {
	if acc == nil {
		return
	}
	syncAccountUsage(acc)
	if acc.Usage == nil {
		return
	}
	if acc.Usage.Type == "unlimited" {
		acc.Usage.Status = "available"
		acc.Usage.Description = "无限空间"
		acc.Usage.UpdatedAt = time.Now().Unix()
		return
	}
	if acc.Usage.Size > 0 {
		acc.Usage.Status = "available"
		acc.Usage.Description = ""
	} else {
		acc.Usage.Status = "unsupported"
		acc.Usage.Description = "暂无容量信息"
	}
	acc.Usage.UpdatedAt = time.Now().Unix()
}

func markQuotaRefreshFailure(acc *model.Account, err error) {
	if acc == nil {
		return
	}
	syncAccountUsage(acc)
	if acc.Usage == nil {
		return
	}
	if acc.Usage.Type == "unlimited" {
		return
	}
	message := strings.ToLower(fmt.Sprint(err))
	for _, marker := range []string{"429", "too many", "rate limit", "risk", "captcha", "风控", "频繁", "限流"} {
		if strings.Contains(message, marker) {
			acc.Usage.Status = "rate_limited"
			acc.Usage.Description = "刷新受限"
			return
		}
	}
	acc.Usage.Status = "error"
	acc.Usage.Description = "刷新失败"
}

func (a *App) accountRefreshCached(userID string) bool {
	now := time.Now()
	a.accountRefreshMu.Lock()
	defer a.accountRefreshMu.Unlock()
	if last := a.accountRefreshLast[userID]; !last.IsZero() && now.Sub(last) < accountRefreshTTL {
		return true
	}
	return now.Before(a.accountRefreshRetryAfter[userID])
}

func (a *App) markAccountRefreshSuccess(userID string) {
	a.accountRefreshMu.Lock()
	defer a.accountRefreshMu.Unlock()
	if a.accountRefreshLast == nil {
		a.accountRefreshLast = make(map[string]time.Time)
	}
	if a.accountRefreshRetryAfter == nil {
		a.accountRefreshRetryAfter = make(map[string]time.Time)
	}
	a.accountRefreshLast[userID] = time.Now()
	delete(a.accountRefreshRetryAfter, userID)
}

func (a *App) markAccountRefreshFailure(userID string, err error) {
	backoff := accountRefreshErrorBackoff
	msg := strings.ToLower(fmt.Sprint(err))
	for _, marker := range []string{"429", "too many", "rate limit", "risk", "captcha", "风控", "频繁", "限流"} {
		if strings.Contains(msg, marker) {
			backoff = accountRefreshRiskBackoff
			break
		}
	}
	var retryAfter retryAfterError
	if errors.As(err, &retryAfter) {
		if requested := retryAfter.RetryAfter(); requested > backoff {
			backoff = requested
		}
	}
	a.accountRefreshMu.Lock()
	defer a.accountRefreshMu.Unlock()
	if a.accountRefreshRetryAfter == nil {
		a.accountRefreshRetryAfter = make(map[string]time.Time)
	}
	a.accountRefreshRetryAfter[userID] = time.Now().Add(backoff)
}

// RefreshAccount silently refreshes an account's quota + profile from the
// provider, persists the updated token, and returns the refreshed account.
// The frontend calls this during startup or an explicit user refresh.
func (a *App) RefreshAccount(userID string) (*model.Account, error) {
	started := time.Now()
	logging.Debug("account refresh started", "account_id", redactID(userID))
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	acc, err := st.GetAccount(userID)
	if err != nil || acc == nil {
		return nil, fmt.Errorf("账号不存在")
	}
	syncAccountUsage(acc)
	if a.accountRefreshCached(userID) {
		return acc, nil
	}
	value, err, _ := a.accountRefreshGroup.Do(userID, func() (any, error) {
		current, getErr := st.GetAccount(userID)
		if getErr != nil || current == nil {
			return nil, fmt.Errorf("账号不存在")
		}
		if a.accountRefreshCached(userID) {
			syncAccountUsage(current)
			return current, nil
		}
		tok, refreshErr := drive.RefreshAccount(userID, current.DriveID)
		if refreshErr != nil {
			a.markAccountRefreshFailure(userID, refreshErr)
			// The provider layer may already have persisted a rotated token even
			// when a later quota request failed. Re-read before saving display
			// status so a stale account object cannot overwrite that token.
			if latest, latestErr := st.GetAccount(userID); latestErr == nil && latest != nil {
				current = latest
			}
			markQuotaRefreshFailure(current, refreshErr)
			if saveErr := st.SaveAccount(current); saveErr != nil {
				logging.Warn("account refresh failure status persistence failed", "account_id", redactID(userID), "error", saveErr)
			}
			a.emit("account:changed", current)
			logging.Warn("account refresh failed", "account_id", redactID(userID), "error", refreshErr, "duration", logging.Duration(started))
			return current, refreshErr
		}
		if tok != nil {
			current.Token = tok
			provider := tok.TokenFrom
			if provider == "" {
				provider = model.ResolveProviderFromUserID(current.UserID)
			}
			accountID := model.StripUserID(provider, tok.UserID)
			if accountID == "" {
				accountID = tok.ProviderAccountID
			}
			current.DriveID = normalizedDriveID(provider, accountID, tok.DefaultDriveID)
		}
		markQuotaRefreshSuccess(current)
		if saveErr := st.SaveAccount(current); saveErr != nil {
			logging.Error("refreshed account persistence failed", "account_id", redactID(userID), "error", saveErr)
			return current, fmt.Errorf("保存账号失败: %w", saveErr)
		}
		a.markAccountRefreshSuccess(userID)
		a.emit("account:changed", current)
		return current, nil
	})
	if value != nil {
		acc = value.(*model.Account)
	}
	if err != nil {
		return acc, err
	}
	logging.Debug("account refresh completed", "account_id", redactID(userID), "duration", logging.Duration(started))
	return acc, nil
}

// RefreshAccountNow bypasses the normal success TTL for startup/manual refresh.
// It keeps server cooldowns and a short per-account manual gap.
func (a *App) RefreshAccountNow(userID string) (*model.Account, error) {
	now := time.Now()
	a.accountRefreshMu.Lock()
	last := a.accountRefreshLast[userID]
	if last.IsZero() || now.Sub(last) >= accountRefreshManualGap {
		delete(a.accountRefreshLast, userID)
	}
	a.accountRefreshMu.Unlock()
	return a.RefreshAccount(userID)
}

// RemoveAccount deletes an account.
func (a *App) RemoveAccount(userID string) error {
	logging.Info("account removal started", "account_id", redactID(userID))
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	if err := st.DeleteAccount(userID); err != nil {
		logging.Warn("account removal failed", "account_id", redactID(userID), "error", err)
		return err
	}
	a.accountRefreshMu.Lock()
	delete(a.accountRefreshLast, userID)
	delete(a.accountRefreshRetryAfter, userID)
	a.accountRefreshMu.Unlock()
	a.emit("account:changed", map[string]string{"removed": userID})
	logging.Info("account removal completed", "account_id", redactID(userID))
	return nil
}
