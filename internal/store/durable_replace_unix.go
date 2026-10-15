//go:build !windows

package store

import (
	"os"
	"path/filepath"
)

func replaceDurable(source, target string) error {
	if err := renameWithRetry(source, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
