package drive

import (
	"encoding/json"

	"mnemo-go/internal/model"
)

func ConditionalCapabilities(caps Capabilities, reason string, requires ...string) AccountCapabilitySnapshot {
	snapshot := AccountCapabilitySnapshot{Overrides: map[string]bool{}, States: map[string]model.CapabilityState{}}
	encoded, _ := json.Marshal(caps)
	values := map[string]any{}
	_ = json.Unmarshal(encoded, &values)
	for _, operation := range []string{"download", "upload", "createFolder", "createTextFile", "createDateFolder", "rename", "move", "copy", "permanentDelete", "trashView", "trashRestore", "trashPurge", "trashClear", "createShare", "importShare", "manageCreatedShares", "cancelCreatedShares", "editCreatedShares", "offlineDownload", "search", "favorite", "quickTransfer", "cloudVideoPreview"} {
		if enabled, ok := values[operation].(bool); ok && enabled {
			snapshot.States[operation] = model.CapabilityState{State: "conditional", Reason: reason, Requires: append([]string(nil), requires...)}
		}
	}
	return snapshot
}

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
	snapshot = ConditionalCapabilities(registration.Caps, "驱动已支持；账号套餐、目标目录权限和平台风控以实际操作结果为准", "valid_session", "remote_permission")
	if provider, ok := registration.Factory().(AccountCapabilityProvider); ok {
		custom := provider.AccountCapabilities(Context{UserID: account.UserID, DriveID: account.DriveID, TokenFrom: account.Provider(), Token: account.Token})
		for key, value := range custom.Overrides {
			snapshot.Overrides[key] = value
		}
		for key, value := range custom.States {
			snapshot.States[key] = value
		}
	}
	if account.Disabled || account.AuthState == "reauth_required" {
		if snapshot.States == nil {
			snapshot.States = map[string]model.CapabilityState{}
		}
		snapshot.States["authentication"] = model.CapabilityState{State: "unsupported", Reason: "账号需要重新授权，已保留离线缓存", Requires: []string{"reauthorize"}}
	}
	return snapshot
}
