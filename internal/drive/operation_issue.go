package drive

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mnemo-go/internal/model"
)

func DescribeOperationError(err error) *model.OperationIssue {
	if err == nil {
		return nil
	}
	issue := &model.OperationIssue{Code: "UNKNOWN", Message: "操作失败，请核对错误详情", Action: "inspect"}
	switch {
	case errors.Is(err, ErrUnauthorized):
		issue.Code, issue.Message, issue.Action = "NEEDS_AUTH", "账号登录已失效，请重新登录后再恢复任务", "reauthorize"
	case errors.Is(err, context.Canceled):
		issue.Code, issue.Message, issue.Action = "USER_CANCELED", "操作已停止，不会自动恢复", "manual_resume"
	case errors.Is(err, syscall.ENOSPC) || (runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(112))):
		issue.Code, issue.Message, issue.Action = "NO_LOCAL_SPACE", "本地存储空间不足，请清理磁盘空间", "check_disk"
	case errors.Is(err, os.ErrPermission):
		issue.Code, issue.Message, issue.Action = "LOCAL_PERMISSION", "本地文件访问受限，请检查目录权限或文件占用", "check_local_path"
	case errors.Is(err, os.ErrNotExist):
		issue.Code, issue.Message, issue.Action = "LOCAL_NOT_FOUND", "本地文件或目录不存在，请检查路径", "check_local_path"
	case errors.Is(err, ErrNotFound):
		issue.Code, issue.Message, issue.Action = "REMOTE_NOT_FOUND", "网盘文件或目录不存在，请刷新确认", "refresh_directory"
	case errors.Is(err, ErrNotImplemented):
		issue.Code, issue.Message, issue.Action = "UNSUPPORTED", "当前网盘不支持此操作", "inspect_capability"
	default:
		var delayed interface{ RetryAfter() time.Duration }
		var status interface{ HTTPStatusCode() int }
		var network net.Error
		switch {
		case errors.As(err, &delayed) && delayed.RetryAfter() > 0:
			issue.Code, issue.Message, issue.Action = "PROVIDER_COOLDOWN", "网盘请求频繁受限，请稍后再试", "wait"
			issue.RetryAt = time.Now().Add(delayed.RetryAfter()).Unix()
		case errors.As(err, &status):
			switch status.HTTPStatusCode() {
			case http.StatusTooManyRequests:
				issue.Code, issue.Message, issue.Action = "PROVIDER_COOLDOWN", "请求过于频繁已被限流，请稍后重试", "wait"
			case http.StatusUnauthorized, http.StatusForbidden:
				issue.Code, issue.Message, issue.Action = "REMOTE_ACCESS_REJECTED", "访问被拒绝，请检查文件权限、链接有效期或账号状态", "check_access"
			case http.StatusNotFound, http.StatusGone:
				issue.Code, issue.Message, issue.Action = "REMOTE_NOT_FOUND", "文件不存在或链接已失效，请刷新确认", "refresh_directory"
			case http.StatusConflict, http.StatusPreconditionFailed:
				issue.Code, issue.Message, issue.Action = "BLOCKED_BY_CONFLICT", "目标位置存在冲突或文件已变动，请检查目录", "check_target"
			case http.StatusRequestTimeout, http.StatusGatewayTimeout:
				issue.Code, issue.Message, issue.Action = "NETWORK_UNAVAILABLE", "网络请求超时，请检查网络或代理设置", "network_settings"
			default:
				if status.HTTPStatusCode() >= 500 && status.HTTPStatusCode() <= 599 {
					issue.Code, issue.Message, issue.Action = "REMOTE_UNAVAILABLE", "网盘服务暂时不可用，请稍后重试", "wait"
				}
			}
		case errors.As(err, &network), errors.Is(err, context.DeadlineExceeded), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			issue.Code, issue.Message, issue.Action = "NETWORK_UNAVAILABLE", "网络连接中断，请检查网络或代理设置", "network_settings"
		}
	}
	if issue.Code == "PROVIDER_COOLDOWN" || issue.Code == "REMOTE_UNAVAILABLE" {
		var header interface{ HTTPRetryAfter() string }
		if errors.As(err, &header) {
			value := strings.TrimSpace(header.HTTPRetryAfter())
			if seconds, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil && seconds > 0 && seconds <= 315360000 {
				issue.RetryAt = time.Now().Add(time.Duration(seconds) * time.Second).Unix()
			} else if deadline, dateErr := http.ParseTime(value); dateErr == nil && deadline.After(time.Now()) {
				issue.RetryAt = deadline.Unix()
			}
		}
	}
	return issue
}
