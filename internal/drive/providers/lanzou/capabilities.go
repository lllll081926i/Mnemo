package lanzou

import (
	"fmt"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) AccountCapabilities(account drive.Context) drive.AccountCapabilitySnapshot {
	snapshot := drive.ConditionalCapabilities(driver.Capabilities(), "操作受有效会话、账号套餐和平台文件规则限制", "valid_session", "provider_file_rules")
	snapshot.States["upload"] = model.CapabilityState{State: "conditional", Reason: fmt.Sprintf("当前配置单文件上限 %d MB；该配置不是平台会员认证，还受文件类型与实际套餐限制", lanzouUploadLimit(lanzouUploadTier(account))/lanzouMiB), Requires: []string{"actual_membership", "allowed_extension", "file_size_limit"}}
	return snapshot
}
