package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestDiagnosticsExportsOnlySafeLocalSummary(t *testing.T) {
	directory := t.TempDir()
	st, err := store.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultSettings()
	settings.DownloadDir = directory
	settings.Proxy = "http://private-proxy:private-password@private-host:8080"
	if err := st.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAccount(&model.Account{UserID: "private-user", DriveID: "private-drive", CustomName: "private-name", AuthState: "reauth_required", Token: &model.TokenInfo{AccessToken: "private-access", RefreshToken: "private-refresh"}}); err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	report, err := application.RunDiagnostics("", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Accounts != 1 || report.Reauthorize != 1 || report.RefreshStored != 1 {
		t.Fatalf("账号状态统计错误：%+v", report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-", directory, "access_token", "refresh_token", "Cookie"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("诊断报告包含敏感信息：%q", secret)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mnemo-diagnostic-") {
			t.Fatal("诊断遗留临时文件")
		}
	}
	if application.diagnostics != report {
		t.Fatal("导出没有保存本次诊断")
	}
	if err := diagnosticDirectoryWrite(directory + "/missing"); err == nil {
		t.Fatal("不存在的下载目录不能显示写入通过")
	}
}
