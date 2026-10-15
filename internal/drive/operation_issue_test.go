package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

type diagnosticHTTPError struct {
	status int
	header string
}

func (failure diagnosticHTTPError) Error() string {
	return "HTTP 错误 https://private.test?token=secret"
}
func (failure diagnosticHTTPError) HTTPStatusCode() int    { return failure.status }
func (failure diagnosticHTTPError) HTTPRetryAfter() string { return failure.header }

type diagnosticCooldownError struct{}

func (diagnosticCooldownError) Error() string             { return "provider risk control" }
func (diagnosticCooldownError) RetryAfter() time.Duration { return 90 * time.Second }

func TestDescribeOperationErrorUsesTypedEvidence(testContext *testing.T) {
	if DescribeOperationError(nil) != nil {
		testContext.Fatal("没有错误时不应生成诊断")
	}
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"授权", AuthExpired("HTTP 403"), "NEEDS_AUTH"},
		{"取消", context.Canceled, "USER_CANCELED"},
		{"空间", &os.PathError{Op: "write", Path: "private-file", Err: syscall.ENOSPC}, "NO_LOCAL_SPACE"},
		{"本机权限", os.ErrPermission, "LOCAL_PERMISSION"},
		{"本机文件", os.ErrNotExist, "LOCAL_NOT_FOUND"},
		{"远端文件", ErrNotFound, "REMOTE_NOT_FOUND"},
		{"能力", NotSupported("upload"), "UNSUPPORTED"},
		{"请求超时", context.DeadlineExceeded, "NETWORK_UNAVAILABLE"},
		{"网络", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, "NETWORK_UNAVAILABLE"},
		{"响应中断", io.ErrUnexpectedEOF, "NETWORK_UNAVAILABLE"},
		{"HTTP401不证明账号过期", diagnosticHTTPError{status: 401}, "REMOTE_ACCESS_REJECTED"},
		{"HTTP403不证明账号过期", diagnosticHTTPError{status: 403}, "REMOTE_ACCESS_REJECTED"},
		{"HTTP404", diagnosticHTTPError{status: 404}, "REMOTE_NOT_FOUND"},
		{"HTTP409", diagnosticHTTPError{status: 409}, "BLOCKED_BY_CONFLICT"},
		{"HTTP412", diagnosticHTTPError{status: 412}, "BLOCKED_BY_CONFLICT"},
		{"HTTP429", diagnosticHTTPError{status: 429}, "PROVIDER_COOLDOWN"},
		{"HTTP503", diagnosticHTTPError{status: 503}, "REMOTE_UNAVAILABLE"},
		{"HTTP504", diagnosticHTTPError{status: 504}, "NETWORK_UNAVAILABLE"},
		{"HTTP400未知", diagnosticHTTPError{status: 400}, "UNKNOWN"},
		{"文件名不是状态码", errors.New("401份资料-429.txt secret-token"), "UNKNOWN"},
		{"权限不是登录失效", errors.New("permission denied token=secret"), "UNKNOWN"},
		{"合并错误保留明确授权证据", errors.Join(diagnosticHTTPError{status: 503}, ErrUnauthorized), "NEEDS_AUTH"},
	}
	for _, sample := range cases {
		testContext.Run(sample.name, func(child *testing.T) {
			issue := DescribeOperationError(fmt.Errorf("operation: %w", sample.err))
			if issue.Code != sample.code || issue.Message == "" || issue.Action == "" {
				child.Fatalf("诊断不符：%+v", issue)
			}
			encoded, err := json.Marshal(issue)
			if err != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "private") {
				child.Fatalf("诊断泄露原始错误：%s %v", encoded, err)
			}
		})
	}
}

func TestOperationCooldownKeepsDeadline(testContext *testing.T) {
	before := time.Now().Unix()
	for _, failure := range []error{diagnosticCooldownError{}, diagnosticHTTPError{status: 429, header: "90"}, diagnosticHTTPError{status: 503, header: time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)}} {
		issue := DescribeOperationError(failure)
		if issue.RetryAt < before+88 || issue.RetryAt > time.Now().Unix()+92 {
			testContext.Fatalf("服务商冷却时间丢失：%+v", issue)
		}
	}
	for _, value := range []string{"", "bad", "-1", "999999999999999999999999999"} {
		issue := DescribeOperationError(diagnosticHTTPError{status: 429, header: value})
		if issue.Code != "PROVIDER_COOLDOWN" || issue.RetryAt != 0 {
			testContext.Fatalf("错误的冷却头不能伪造恢复时间：%+v", issue)
		}
	}
}
