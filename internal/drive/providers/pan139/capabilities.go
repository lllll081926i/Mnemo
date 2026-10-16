package pan139

import (
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) AccountCapabilities(account drive.Context) drive.AccountCapabilitySnapshot {
	snapshot := drive.ConditionalCapabilities(driver.Capabilities(), "个人云与家庭云权限分别判断；操作需要目标空间、目录权限及有效会话", "space_membership", "target_directory_permission")
	snapshot.States["createShare"] = model.CapabilityState{State: "conditional", Reason: "仅个人云支持创建公开分享；家庭云文件不能创建公开分享", Requires: []string{"personal_cloud_file"}}
	snapshot.States["familyCloud"] = model.CapabilityState{State: "conditional", Reason: "家庭云需要有效成员身份及平台返回的家庭标识；尚未验证当前目录写权限", Requires: []string{"family_membership", "family_cloud_id"}}
	if pan139FamilyUnavailable(account) {
		snapshot.States["familyCloud"] = model.CapabilityState{State: "unsupported", Reason: "平台最近未返回可用家庭云；个人云功能不受影响，请稍后刷新家庭云"}
	}
	return snapshot
}
