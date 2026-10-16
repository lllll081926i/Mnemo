package s3

import (
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) AccountCapabilities(account drive.Context) drive.AccountCapabilitySnapshot {
	snapshot := drive.ConditionalCapabilities(driver.Capabilities(), "S3 的目录浏览不代表拥有写权限；每个操作由实例权限及目标路径决定", "endpoint_permission", "target_path_permission")
	snapshot.States["authentication"] = model.CapabilityState{State: "conditional", Reason: "连接成功仅证明当时可浏览配置的根目录；不代表令牌始终有效或可访问所有目录", Requires: []string{"valid_credentials", "configured_root"}}
	snapshot.States["upload"] = model.CapabilityState{State: "conditional", Reason: "上传、覆盖和删除权限相互独立；覆盖还需要服务端正确执行条件写入", Requires: []string{"target_write_permission", "server_conditional_write"}}
	return snapshot
}
