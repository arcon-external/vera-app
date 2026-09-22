package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/labstack/echo/v4"
	"google.golang.org/api/iterator"
)

// maxBackupDownload caps a streamed dump at 512MB. Production dumps are a
// few MB; anything larger is almost certainly a mistake.
const maxBackupDownload = 512 << 20

// backupSource yields the newest available dump. The directory source serves
// local files (tests, local dev); the bucket source serves object storage.
type backupSource interface {
	latest(ctx context.Context) (name string, data []byte, err error)
}

// backupSink receives a safety snapshot. When no sink is configured the
// staged file on disk IS the snapshot (local dev, tests); the bucket sink
// uploads to object storage so the rollback artifact outlives the instance.
type backupSink interface {
	put(ctx context.Context, name string, data []byte) error
}

// SetBackupSink overrides the snapshot sink (tests use a fake).
func (s *Server) SetBackupSink(sink backupSink) {
	s.sink = sink
}

// gcsBackupSink uploads snapshots to a bucket under their plain names, so
// the existing latest-backup endpoint (prefix vera-*.dump) can serve them.
type gcsBackupSink struct {
	bucket string
}

func (g *gcsBackupSink) put(ctx context.Context, name string, data []byte) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("open backup storage: %w", err)
	}
	defer client.Close()
	writer := client.Bucket(g.bucket).Object(name).NewWriter(ctx)
	writer.ContentType = "application/octet-stream"
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write snapshot object: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish snapshot object: %w", err)
	}
	return nil
}

// snapshotSink resolves the configured sink: explicit override first, then
// the bucket when VERA_BACKUP_BUCKET is set, else nil for local-file
// snapshots. The bucket name is returned for bucket-addressed messages.
func (s *Server) snapshotSink() (backupSink, string) {
	if s.sink != nil {
		return s.sink, ""
	}
	if bucket := os.Getenv("VERA_BACKUP_BUCKET"); bucket != "" {
		return &gcsBackupSink{bucket: bucket}, bucket
	}
	return nil, ""
}

// SetBackupSource overrides the backup source (tests and operators).
func (s *Server) SetBackupSource(source backupSource) {
	s.backups = source
}

// dirBackupSource serves vera-*.dump files from a directory, newest first.
type dirBackupSource struct {
	dir string
}

func (d *dirBackupSource) latest(ctx context.Context) (string, []byte, error) {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return "", nil, fmt.Errorf("read backup directory: %w", err)
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
		dumps = append(dumps, candidate{filepath.Join(d.dir, name), info.ModTime().UnixNano()})
	}
	if len(dumps) == 0 {
		return "", nil, fmt.Errorf("no backups available")
	}
	sort.Slice(dumps, func(i, j int) bool { return dumps[i].modTime > dumps[j].modTime })
	data, err := os.ReadFile(dumps[0].path)
	if err != nil {
		return "", nil, fmt.Errorf("read newest dump: %w", err)
	}
	return filepath.Base(dumps[0].path), data, nil
}

// gcsBackupSource serves vera-*.dump objects from a bucket, newest first.
type gcsBackupSource struct {
	bucket string
}

func (g *gcsBackupSource) latest(ctx context.Context) (string, []byte, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("open backup storage: %w", err)
	}
	defer client.Close()
	query := &storage.Query{Prefix: "vera-"}
	it := client.Bucket(g.bucket).Objects(ctx, query)
	var newestName string
	var newestTime int64
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("list backups: %w", err)
		}
		if !strings.HasSuffix(attrs.Name, ".dump") {
			continue
		}
		if timestamp := attrs.Updated.UnixNano(); timestamp > newestTime {
			newestTime = timestamp
			newestName = attrs.Name
		}
	}
	if newestName == "" {
		return "", nil, fmt.Errorf("no backups available")
	}
	reader, err := client.Bucket(g.bucket).Object(newestName).NewReader(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("open newest dump: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxBackupDownload+1))
	if err != nil {
		return "", nil, fmt.Errorf("read newest dump: %w", err)
	}
	if len(data) > maxBackupDownload {
		return "", nil, fmt.Errorf("newest dump exceeds 512MB")
	}
	return newestName, data, nil
}

func (s *Server) downloadBackup(context echo.Context) error {
	source := s.backups
	if source == nil {
		if bucket := os.Getenv("VERA_BACKUP_BUCKET"); bucket != "" {
			source = &gcsBackupSource{bucket: bucket}
		} else {
			source = &dirBackupSource{dir: s.backupDirOrDefault()}
		}
	}
	name, data, err := source.latest(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "no backups available yet"})
	}
	context.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filepath.Base(name)))
	return context.Blob(http.StatusOK, "application/octet-stream", data)
}
