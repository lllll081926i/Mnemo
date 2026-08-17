package app

import (
	"os"
	"os/exec"
	"path/filepath"
	gruntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PickDirectory opens the native OS directory picker and returns the chosen
// path. Returns "" when the user cancels. defaultDir is used only when it
// exists on disk.
func (a *App) PickDirectory(title, defaultDir string) string {
	opts := runtime.OpenDialogOptions{
		Title:                title,
		CanCreateDirectories: true,
	}
	if defaultDir != "" {
		if st, err := os.Stat(defaultDir); err == nil && st.IsDir() {
			opts.DefaultDirectory = defaultDir
		}
	}
	ctx, ok := a.wailsContext()
	if !ok {
		return ""
	}
	dir, err := runtime.OpenDirectoryDialog(ctx, opts)
	if err != nil {
		return ""
	}
	return dir
}

// PickFiles opens the native OS multi-file picker and returns chosen paths.
// Returns nil when the user cancels.
func (a *App) PickFiles(title string) []string {
	ctx, ok := a.wailsContext()
	if !ok {
		return nil
	}
	files, err := runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{Title: title})
	if err != nil {
		return nil
	}
	return files
}

// RevealInFolder opens the OS file manager at the containing folder of path,
// selecting the file where the platform supports it.
func (a *App) RevealInFolder(path string) {
	if path == "" {
		return
	}
	switch gruntime.GOOS {
	case "windows":
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			_ = exec.Command("explorer.exe", path).Start()
		} else {
			_ = exec.Command("explorer.exe", "/select,", path).Start()
		}
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	default: // linux 无标准“选中文件”，打开所在目录
		dir := path
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			dir = filepath.Dir(path)
		}
		_ = exec.Command("xdg-open", dir).Start()
	}
}

// OpenFile opens a file with the OS default application.
func (a *App) OpenFile(path string) {
	if path == "" {
		return
	}
	switch gruntime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		_ = exec.Command("open", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
	}
}
