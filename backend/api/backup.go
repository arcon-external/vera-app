package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
)

// latestBackup returns the newest vera-*.dump file in dir.
func latestBackup(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read backup directory: %w", err)
	}
	type candidate struct {
		path    string
		modTime int64
	}
	var dumps []candidate
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "vera-") || !strings.HasSuffix(name, ".dump") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		dumps = append(dumps, candidate{filepath.Join(dir, name), info.ModTime().UnixNano()})
	}
	if len(dumps) == 0 {
		return "", fmt.Errorf("no backups available")
	}
	sort.Slice(dumps, func(i, j int) bool { return dumps[i].modTime > dumps[j].modTime })
	return dumps[0].path, nil
}

func (s *Server) downloadBackup(context echo.Context) error {
	path, err := latestBackup(s.backupDirOrDefault())
	if err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "no backups available yet"})
	}
	return context.Attachment(path, filepath.Base(path))
}
