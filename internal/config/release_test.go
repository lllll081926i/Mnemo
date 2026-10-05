package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestReleaseVersionConsistency(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, name := range []string{"wails.json", "frontend/package.json", "frontend/package-lock.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Version string `json:"version"`
			Info    struct {
				ProductVersion string `json:"productVersion"`
			} `json:"info"`
			Packages map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Version != AppVersion || name == "wails.json" && manifest.Info.ProductVersion != AppVersion || name == "frontend/package-lock.json" && manifest.Packages[""].Version != AppVersion {
			t.Fatalf("%s 的版本与应用版本 %s 不一致", name, AppVersion)
		}
	}
	installer, err := os.ReadFile(filepath.Join(root, "build", "windows", "installer.iss"))
	if err != nil {
		t.Fatal(err)
	}
	version := regexp.MustCompile(`(?m)^\s*#define MyAppVersion "([^"]+)"`).FindSubmatch(installer)
	if len(version) != 2 || string(version[1]) != AppVersion {
		t.Fatal("Windows 安装器版本与应用版本不一致")
	}
	if info, err := os.Stat(filepath.Join(root, "docs", "releases", "v"+AppVersion+".md")); err != nil || !info.Mode().IsRegular() {
		t.Fatal("当前版本缺少正式更新说明")
	}
}
