package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/netx"
)

type DiagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type DiagnosticReport struct {
	Version         string            `json:"version"`
	Platform        string            `json:"platform"`
	GoVersion       string            `json:"goVersion"`
	CreatedAt       int64             `json:"createdAt"`
	ProxyConfigured bool              `json:"proxyConfigured"`
	Accounts        int               `json:"accounts"`
	Reauthorize     int               `json:"reauthorize"`
	Disabled        int               `json:"disabled"`
	RefreshStored   int               `json:"refreshStored"`
	Checks          []DiagnosticCheck `json:"checks"`
}

func (a *App) RunDiagnostics(userID, driveID string, network bool) (*DiagnosticReport, error) {
	if !a.diagnosticsMu.TryLock() {
		return nil, errors.New("诊断正在运行，请稍后再试")
	}
	defer a.diagnosticsMu.Unlock()
	st, err := a.storeOrError()
	if err != nil {
		return nil, errors.New("本机存储无法读取，请检查用户数据目录权限")
	}
	accounts, err := st.ListAccounts()
	if err != nil {
		return nil, errors.New("账号记录无法读取，请检查本机凭据存储；不要删除密钥文件")
	}
	report := &DiagnosticReport{Version: config.AppVersion, Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), CreatedAt: time.Now().Unix(), ProxyConfigured: netx.EffectiveProxyURL() != "", Accounts: len(accounts)}
	selected, disabled := false, false
	for _, account := range accounts {
		if account.AuthState == "reauth_required" {
			report.Reauthorize++
		}
		if account.Disabled {
			report.Disabled++
		}
		if account.Token != nil && (account.Token.RefreshToken != "" || account.Token.OpenAPIRefreshToken != "") {
			report.RefreshStored++
		}
		if account.UserID == userID && account.DriveID == driveID {
			selected, disabled = true, account.Disabled
		}
	}
	add := func(name, status, code, message string) {
		report.Checks = append(report.Checks, DiagnosticCheck{Name: name, Status: status, Code: code, Message: message})
	}
	add("账号生命周期", "info", "LOCAL_AUTH_SNAPSHOT", fmt.Sprintf("%d 个账号需要重新登录；%d 个账号保存了刷新凭据。刷新凭据存在不代表服务端仍然接受，未主动刷新所有账号。", report.Reauthorize, report.RefreshStored))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		add("OAuth 本机回调", "failed", "CALLBACK_BIND_FAILED", "本机回环端口无法监听，请检查防火墙与安全软件；未验证 OAuth 应用配置。")
	} else {
		_ = listener.Close()
		add("OAuth 本机回调", "passed", "CALLBACK_BIND_OK", "本机回环端口可以监听；这不代表服务商已批准 OAuth 应用或回调地址。")
	}
	directory, err := a.GetDownloadDirectory()
	if err == nil {
		err = diagnosticDirectoryWrite(directory)
	}
	if err != nil {
		issue := drive.DescribeOperationError(err)
		add("下载目录写入", "failed", issue.Code, "无法完成临时文件写入，请检查下载目录是否存在、剩余空间、写权限与安全软件。")
	} else {
		add("下载目录写入", "passed", "LOCAL_WRITE_OK", "临时文件写入和同步成功，不代表磁盘有足够空间完成全部下载。")
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 25*time.Second)
	defer cancel()
	if network {
		for _, check := range diagnosticNetwork(ctx) {
			report.Checks = append(report.Checks, check)
		}
	} else {
		add("公共网络", "skipped", "NETWORK_NOT_REQUESTED", "尚未发起 DNS、TCP 或 HTTPS 检测。勾选公共网络检测后重试。")
	}
	if userID == "" {
		add("网盘只读权限", "skipped", "ACCOUNT_NOT_SELECTED", "选择一个账号后可读取根目录第一页。不会上传、删除、分享或恢复任务。")
	} else if !selected || disabled {
		add("网盘只读权限", "failed", "ACCOUNT_UNAVAILABLE", "所选账号已移除、空间已变更或账号已停用，请重新选择。")
	} else {
		root, rootErr := drive.RootID(userID, driveID)
		if rootErr == nil {
			_, rootErr = drive.ListDirPageContext(ctx, userID, driveID, root, "", nil)
		}
		if rootErr != nil {
			issue := drive.DescribeOperationError(rootErr)
			add("网盘只读权限", "failed", issue.Code, issue.Message)
		} else {
			add("网盘只读权限", "passed", "ROOT_READ_OK", "根目录第一页读取成功，不代表上传、删除、套餐、其他目录或分享权限可用。")
		}
	}
	add("Range 与写入权限", "skipped", "RESOURCE_NOT_PROBED", "Range 取决于具体文件直链，以下载任务实际探测为准。没有文件链接时不猜测；未进行云端写入测试。")
	a.diagnostics = report
	return report, nil
}

func diagnosticDirectoryWrite(directory string) error {
	file, err := os.CreateTemp(directory, ".mnemo-diagnostic-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write([]byte("Mnemo diagnostics\n"))
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func diagnosticNetwork(parent context.Context) []DiagnosticCheck {
	checks := []DiagnosticCheck{}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	_, err := net.DefaultResolver.LookupHost(ctx, "api.github.com")
	cancel()
	if err != nil {
		checks = append(checks, DiagnosticCheck{"本机 DNS", "failed", "DNS_FAILED", "本机无法解析公共测试域名；代理可能使用远端 DNS，请继续检查 HTTPS 结果。"})
	} else {
		checks = append(checks, DiagnosticCheck{"本机 DNS", "passed", "DNS_OK", "公共测试域名解析成功，不代表网盘域名均可解析。"})
	}
	address, name := "api.github.com:443", "公共 TCP"
	if proxy := netx.EffectiveProxyURL(); proxy != "" {
		parsed, parseErr := url.Parse(proxy)
		if parseErr != nil || parsed.Hostname() == "" {
			return append(checks, DiagnosticCheck{"代理配置", "failed", "INVALID_PROXY", "代理地址无效，请检查网络设置。导出不会包含代理地址或密码。"})
		}
		port := parsed.Port()
		if port == "" {
			switch parsed.Scheme {
			case "https":
				port = "443"
			case "socks5", "socks5h":
				port = "1080"
			default:
				port = "80"
			}
		}
		address, name = net.JoinHostPort(parsed.Hostname(), port), "代理 TCP"
	}
	ctx, cancel = context.WithTimeout(parent, 5*time.Second)
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	cancel()
	if err != nil {
		checks = append(checks, DiagnosticCheck{name, "failed", "TCP_FAILED", "TCP 连接失败，请检查网络、代理端口与防火墙。"})
	} else {
		_ = connection.Close()
		checks = append(checks, DiagnosticCheck{name, "passed", "TCP_OK", "TCP 连接成功，不代表代理认证或网盘 API 可用。"})
	}
	client := netx.NewClientWithSystemProxy(8 * time.Second).HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer client.CloseIdleConnections()
	request, _ := http.NewRequestWithContext(parent, http.MethodGet, "https://api.github.com/rate_limit", nil)
	request.Header.Set("User-Agent", "Mnemo-Diagnostics")
	response, err := client.Do(request)
	if err != nil {
		checks = append(checks, DiagnosticCheck{"公共 HTTPS", "failed", "HTTPS_FAILED", "公共 HTTPS 请求失败，请检查代理认证、系统时间或证书信任；不要关闭 TLS 证书校验。"})
	} else {
		_ = response.Body.Close()
		status, code := "passed", "HTTPS_OK"
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			status, code = "failed", "HTTPS_REJECTED"
		}
		checks = append(checks, DiagnosticCheck{"公共 HTTPS", status, code, fmt.Sprintf("公共测试端点返回 HTTP %d；这不是网盘请求配额或网盘 API 检测。", response.StatusCode)})
	}
	return checks
}

func (a *App) ExportDiagnostics() (string, error) {
	a.diagnosticsMu.Lock()
	report := a.diagnostics
	a.diagnosticsMu.Unlock()
	if report == nil {
		return "", errors.New("请先运行诊断")
	}
	ctx, ok := a.wailsContext()
	if !ok {
		return "", errors.New("应用窗口尚未初始化")
	}
	destination, err := wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{Title: "导出脱敏诊断", DefaultFilename: "mnemo-diagnostics.json", Filters: []wailsruntime.FileFilter{{DisplayName: "JSON 诊断文件", Pattern: "*.json"}}})
	if err != nil || destination == "" {
		return "", err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		return "", errors.New("诊断文件保存失败，请检查所选目录权限")
	}
	return destination, nil
}
