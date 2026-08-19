package updater

import (
	"fmt"
	"os/exec"
	"runtime"
)

// applyWindows runs the Inno Setup installer silently and lets it restart
// the application.
func applyWindows(installerPath string) error {
	cmd := exec.Command(installerPath, "/SILENT", "/NORESTART", "/CLOSEAPPLICATIONS", "/RESTARTAPPLICATIONS")
	return cmd.Start()
}

// applyUnix extracts the tar.gz in place. For mac/linux the archive contains
// the app bundle/binary; we extract and let the user restart.
func applyUnix(archivePath string) error {
	_ = runtime.GOOS
	// Replacing a running Wails binary/app bundle safely requires a platform
	// specific handoff process. Never report success while leaving the archive
	// untouched; the caller can surface this actionable error instead.
	return fmt.Errorf("automatic update installation is not supported on this platform; downloaded archive: %s", archivePath)
}
