package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateSavePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	if !strings.EqualFold(filepath.Ext(absPath), ".pdf") {
		return "", fmt.Errorf("file must use .pdf extension")
	}

	parentDir := filepath.Dir(absPath)
	parentInfo, err := os.Stat(parentDir)
	if err != nil {
		return "", fmt.Errorf("invalid destination directory: %w", err)
	}
	if !parentInfo.IsDir() {
		return "", fmt.Errorf("destination parent is not a directory")
	}

	fileInfo, err := os.Lstat(absPath)
	if err == nil {
		if fileInfo.IsDir() {
			return "", fmt.Errorf("destination is a directory")
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("destination cannot be a symlink")
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check destination: %w", err)
	}

	return absPath, nil
}
