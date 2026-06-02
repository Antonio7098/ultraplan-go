package study

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type fileSnapshot map[string]string

func snapshotFiles(root string) (fileSnapshot, error) {
	root = filepath.Clean(root)
	snapshot := fileSnapshot{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		snapshot[filepath.Clean(path)] = hash
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func snapshotFilesSettled(root string) (fileSnapshot, error) {
	const (
		settleChecks = 4
		settleDelay  = 250 * time.Millisecond
	)
	previous, err := snapshotFiles(root)
	if err != nil {
		return nil, err
	}
	for range settleChecks {
		time.Sleep(settleDelay)
		next, err := snapshotFiles(root)
		if err != nil {
			return nil, err
		}
		if snapshotsEqual(previous, next) {
			return next, nil
		}
		previous = next
	}
	return previous, nil
}

func snapshotsEqual(a, b fileSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for path, hash := range a {
		if b[path] != hash {
			return false
		}
	}
	return true
}

func unexpectedEditWarnings(root string, before, after fileSnapshot, allowedPaths []string) []string {
	allowed := map[string]struct{}{}
	for _, path := range allowedPaths {
		if path == "" {
			continue
		}
		allowed[filepath.Clean(path)] = struct{}{}
	}
	changed := map[string]string{}
	for path, beforeHash := range before {
		afterHash, ok := after[path]
		switch {
		case !ok:
			changed[path] = "deleted"
		case beforeHash != afterHash:
			changed[path] = "modified"
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changed[path] = "created"
		}
	}
	paths := make([]string, 0, len(changed))
	for path := range changed {
		if _, ok := allowed[path]; ok {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	warnings := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = path
		}
		warnings = append(warnings, fmt.Sprintf("unexpected edit outside allowed paths: %s %s", changed[path], filepath.ToSlash(rel)))
	}
	return warnings
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
