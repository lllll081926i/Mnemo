package registrycheck

import (
	"path/filepath"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestEveryUploadDriverMarksAttemptAfterPreflight(t *testing.T) {
	for _, registration := range drive.All() {
		t.Run(registration.ID, func(t *testing.T) {
			driver := registration.Factory()
			managed, ok := driver.(drive.UploadAttemptDriver)
			if !ok || !managed.ManagesUploadAttempt() {
				t.Fatal("驱动必须明确标记首次写入时机")
			}
			ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: filepath.Join(t.TempDir(), "missing.txt"), Name: "missing.txt"}}
			ui.ConfigureUploadAttempt(func() error { t.Fatal("本地文件或登录检查失败前不得标记远端提交"); return nil })
			if err := driver.UploadOneFile(t.Context(), drive.Context{}, ui); err == nil {
				t.Fatal("缺少凭据或本地文件不能成功")
			}
		})
	}
}
