package drive

import "mnemo-go/internal/model"

type AccountCapabilitySnapshot struct {
	Overrides map[string]bool                  `json:"overrides"`
	States    map[string]model.CapabilityState `json:"states"`
}

type AccountCapabilityProvider interface {
	AccountCapabilities(Context) AccountCapabilitySnapshot
}

func CapabilitiesForAccount(account *model.Account) AccountCapabilitySnapshot {
	snapshot := AccountCapabilitySnapshot{Overrides: map[string]bool{}, States: map[string]model.CapabilityState{}}
	if account == nil {
		return snapshot
	}
	registration, exists := Get(account.Provider())
	if !exists || registration.Factory == nil {
		return snapshot
	}
	if provider, ok := registration.Factory().(AccountCapabilityProvider); ok {
		snapshot = provider.AccountCapabilities(Context{UserID: account.UserID, DriveID: account.DriveID, TokenFrom: account.Provider(), Token: account.Token})
	}
	if account.Disabled || account.AuthState == "reauth_required" {
		if snapshot.States == nil {
			snapshot.States = map[string]model.CapabilityState{}
		}
		snapshot.States["authentication"] = model.CapabilityState{State: "unsupported", Reason: "账号需要重新授权，已保留离线缓存", Requires: []string{"reauthorize"}}
	}
	return snapshot
}
