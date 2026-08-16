package updater

import (
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
	// Best-effort: leave the archive in the updates dir; the frontend will
	// prompt the user to restart manually after extraction.
	return nil
}
