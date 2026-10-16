package pan189

import (
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) AccountCapabilities(account drive.Context) drive.AccountCapabilitySnapshot {
	snapshot := drive.ConditionalCapabilities(driver.Capabilities(), "操作需要对应个人云或家庭云会话、成员身份和目标目录权限", "space_session", "target_directory_permission")
	snapshot.States["createShare"] = model.CapabilityState{State: "conditional", Reason: "仅个人云文件支持创建公开分享；家庭云文件不支持", Requires: []string{"personal_cloud_file"}}
	snapshot.States["familyCloud"] = model.CapabilityState{State: "conditional", Reason: "家庭云需要平台返回的家庭标识及独立会话；浏览成功不代表拥有写权限", Requires: []string{"family_membership", "family_session"}}
	return snapshot
}
