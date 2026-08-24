package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

const (
	openCodeLogMaxAge   = 48 * time.Hour
	openCodeLogMaxBytes = uint64(128 * 1024 * 1024)
)

type logFile struct {
	path    string
	size    uint64
	updated time.Time
}

func pruneOpenCodeLogs(c config.Config) error {
	dataRoot := envValue(c.Agentwrap.Env, "XDG_DATA_HOME")
	if dataRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataRoot = filepath.Join(home, ".local", "share")
	}
	return pruneLogDirectory(filepath.Join(dataRoot, "opencode", "log"), time.Now().UTC(), openCodeLogMaxAge, openCodeLogMaxBytes)
}

func pruneLogDirectory(root string, now time.Time, maxAge time.Duration, maxBytes uint64) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	files := make([]logFile, 0, len(entries))
	var total uint64
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		size := uint64(max(info.Size(), 0))
		files = append(files, logFile{path: filepath.Join(root, entry.Name()), size: size, updated: info.ModTime()})
		total += size
	}
	sort.Slice(files, func(i, j int) bool { return files[i].updated.Before(files[j].updated) })
	var errs []error
	for _, file := range files {
		expired := maxAge > 0 && now.Sub(file.updated) > maxAge
		// Never quota-delete a file OpenCode may still be appending to.
		overQuota := maxBytes > 0 && total > maxBytes && now.Sub(file.updated) > 10*time.Minute
		if !expired && !overQuota {
			continue
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		if file.size <= total {
			total -= file.size
		}
	}
	return errors.Join(errs...)
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimSpace(strings.TrimPrefix(env[i], prefix))
		}
	}
	return ""
}
