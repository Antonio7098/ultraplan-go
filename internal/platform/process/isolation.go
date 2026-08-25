package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IsolationLimits bounds all work performed while materializing a disposable
// workspace. Zero values are invalid so callers cannot accidentally request an
// unbounded copy.
type IsolationLimits struct {
	MaxFiles    int
	MaxBytes    int64
	MaxFileSize int64
	Timeout     time.Duration
}

// IsolationRequest describes one disposable local workspace. ProtectedRoots
// are never copied and are returned as capability facts for product policy.
type IsolationRequest struct {
	SourceRoot     string
	ParentDir      string
	Prefix         string
	ProtectedRoots []string
	Limits         IsolationLimits
}

type IsolationCapabilities struct {
	PrivateWorkspace        bool `json:"private_workspace"`
	ContainedCopy           bool `json:"contained_copy"`
	ProcessGroup            bool `json:"process_group"`
	DescendantCleanup       bool `json:"descendant_cleanup"`
	WorkspaceRemoval        bool `json:"workspace_removal"`
	NativeProtectedRootDeny bool `json:"native_protected_root_deny"`
}

type TreeIdentity struct {
	Digest string `json:"digest"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

type treeEntryIdentity struct {
	digest string
	mode   fs.FileMode
}

type IsolationWorkspace struct {
	Path         string
	Source       TreeIdentity
	Capabilities IsolationCapabilities
	CreatedAt    time.Time
}

type CleanupResult struct {
	Attempted bool
	Complete  bool
	Error     string
}

// IsolationCapabilityFacts probes the host primitives used by disposable
// writable workspaces. Callers must still fail closed when a required fact is
// false.
func IsolationCapabilityFacts() IsolationCapabilities { return isolationCapabilities() }

// CreateIsolation copies a regular local tree into a new private directory.
// It does not depend on Git and rejects links, special files, hard links, path
// escapes, and trees outside the caller's declared limits.
func CreateIsolation(ctx context.Context, req IsolationRequest) (IsolationWorkspace, error) {
	if ctx == nil {
		return IsolationWorkspace{}, fmt.Errorf("isolation context is required")
	}
	if err := validateIsolationRequest(req); err != nil {
		return IsolationWorkspace{}, err
	}
	source, err := filepath.Abs(req.SourceRoot)
	if err != nil {
		return IsolationWorkspace{}, fmt.Errorf("resolve isolation source: %w", err)
	}
	info, err := os.Lstat(source)
	if err != nil {
		return IsolationWorkspace{}, fmt.Errorf("inspect isolation source: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return IsolationWorkspace{}, fmt.Errorf("isolation source must be a real directory")
	}
	parent, err := filepath.Abs(req.ParentDir)
	if err != nil {
		return IsolationWorkspace{}, fmt.Errorf("resolve isolation parent: %w", err)
	}
	if err := rejectRootOverlap(source, parent, req.ProtectedRoots); err != nil {
		return IsolationWorkspace{}, err
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return IsolationWorkspace{}, fmt.Errorf("create isolation parent: %w", err)
	}
	root, err := os.MkdirTemp(parent, safePrefix(req.Prefix))
	if err != nil {
		return IsolationWorkspace{}, fmt.Errorf("create isolation workspace: %w", err)
	}
	created := IsolationWorkspace{Path: root, CreatedAt: time.Now().UTC(), Capabilities: isolationCapabilities()}
	copyCtx, cancel := context.WithTimeout(ctx, req.Limits.Timeout)
	defer cancel()
	identity, err := copyBoundedTree(copyCtx, source, root, req.Limits)
	if err != nil {
		cleanupErr := os.RemoveAll(root)
		if cleanupErr != nil {
			return IsolationWorkspace{}, errors.Join(err, fmt.Errorf("remove failed isolation workspace: %w", cleanupErr))
		}
		return IsolationWorkspace{}, err
	}
	created.Source = identity
	return created, nil
}

func (w IsolationWorkspace) Resolve(rel string) (string, error) {
	if strings.TrimSpace(w.Path) == "" {
		return "", fmt.Errorf("isolation workspace path is required")
	}
	if filepath.IsAbs(rel) || rel == "" {
		return "", fmt.Errorf("isolated path must be non-empty and relative")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("isolated path escapes workspace")
	}
	resolved := filepath.Join(w.Path, clean)
	relToRoot, err := filepath.Rel(w.Path, resolved)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("isolated path escapes workspace")
	}
	return resolved, nil
}

func (w IsolationWorkspace) Run(ctx context.Context, runner Runner, relDir string, req Request) (Result, error) {
	if runner == nil {
		return Result{}, fmt.Errorf("isolated process runner is required")
	}
	dir, err := w.Resolve(relDir)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("isolated process directory is unavailable")
	}
	req.Dir = dir
	for _, entry := range req.Env {
		if strings.ContainsRune(entry, '\x00') || !strings.Contains(entry, "=") {
			return Result{}, fmt.Errorf("isolated process environment is malformed")
		}
	}
	if w.Capabilities.NativeProtectedRootDeny {
		req, err = nativeIsolationRequest(w.Path, dir, req)
		if err != nil {
			return Result{}, err
		}
	}
	return runner.Run(ctx, req)
}

func (w IsolationWorkspace) Identity(ctx context.Context, limits IsolationLimits) (TreeIdentity, error) {
	return IdentifyTree(ctx, w.Path, limits)
}

// IdentifyTree returns a bounded identity without following links or relying
// on Git. It uses the same path-and-content digest as CreateIsolation.
func IdentifyTree(ctx context.Context, root string, limits IsolationLimits) (TreeIdentity, error) {
	identity, _, err := collectTreeIdentity(ctx, root, limits)
	return identity, err
}

// CompareTrees reports regular-file changes between two bounded trees.
func CompareTrees(ctx context.Context, beforeRoot, afterRoot string, limits IsolationLimits) ([]string, error) {
	_, before, err := collectTreeIdentity(ctx, beforeRoot, limits)
	if err != nil {
		return nil, err
	}
	_, after, err := collectTreeIdentity(ctx, afterRoot, limits)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		seen[path] = struct{}{}
	}
	for path := range after {
		seen[path] = struct{}{}
	}
	changed := make([]string, 0)
	for path := range seen {
		left, leftOK := before[path]
		right, rightOK := after[path]
		if !leftOK || !rightOK || left != right {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func (w IsolationWorkspace) Cleanup() CleanupResult {
	result := CleanupResult{Attempted: true}
	if strings.TrimSpace(w.Path) == "" {
		result.Error = "workspace path is empty"
		return result
	}
	if err := os.RemoveAll(w.Path); err != nil {
		result.Error = err.Error()
		return result
	}
	if _, err := os.Lstat(w.Path); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Error = "workspace still exists"
		}
		return result
	}
	result.Complete = true
	return result
}

func validateIsolationRequest(req IsolationRequest) error {
	if strings.TrimSpace(req.SourceRoot) == "" || strings.TrimSpace(req.ParentDir) == "" {
		return fmt.Errorf("isolation source and parent are required")
	}
	if req.Limits.MaxFiles <= 0 || req.Limits.MaxBytes <= 0 || req.Limits.MaxFileSize <= 0 || req.Limits.Timeout <= 0 {
		return fmt.Errorf("isolation limits must be positive")
	}
	if req.Limits.MaxFileSize > req.Limits.MaxBytes {
		return fmt.Errorf("isolation file limit cannot exceed tree byte limit")
	}
	return nil
}

func rejectRootOverlap(source, parent string, protected []string) error {
	for _, raw := range append(append([]string(nil), protected...), source) {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		root, err := filepath.Abs(raw)
		if err != nil {
			return fmt.Errorf("resolve protected root: %w", err)
		}
		if pathsOverlap(root, parent) {
			return fmt.Errorf("isolation parent overlaps source or protected root")
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return true
	}
	rel, err = filepath.Rel(b, a)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func safePrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "ultraplan-isolation-"
	}
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	return value + "-"
}

func copyBoundedTree(ctx context.Context, source, target string, limits IsolationLimits) (TreeIdentity, error) {
	hash := sha256.New()
	identity := TreeIdentity{}
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("copy path escapes source")
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported isolation entry %q", filepath.ToSlash(rel))
		}
		if info.Mode().IsRegular() && info.Sys() != nil && linkCount(info) > 1 {
			return fmt.Errorf("hard-linked isolation entry %q is not allowed", filepath.ToSlash(rel))
		}
		destination := filepath.Join(target, rel)
		if info.IsDir() {
			return os.Mkdir(destination, info.Mode().Perm()&0o700)
		}
		identity.Files++
		identity.Bytes += info.Size()
		if identity.Files > limits.MaxFiles || identity.Bytes > limits.MaxBytes || info.Size() > limits.MaxFileSize {
			return fmt.Errorf("isolation tree exceeds declared limits")
		}
		data, err := readRegularFile(path, info.Size(), limits.MaxFileSize)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, info.Mode().Perm()&0o700); err != nil {
			return err
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(rel))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return TreeIdentity{}, fmt.Errorf("copy isolation tree: %w", err)
	}
	identity.Digest = hex.EncodeToString(hash.Sum(nil))
	return identity, nil
}

func collectTreeIdentity(ctx context.Context, root string, limits IsolationLimits) (TreeIdentity, map[string]treeEntryIdentity, error) {
	if ctx == nil || limits.MaxFiles <= 0 || limits.MaxBytes <= 0 || limits.MaxFileSize <= 0 {
		return TreeIdentity{}, nil, fmt.Errorf("tree identity requires a context and positive limits")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return TreeIdentity{}, nil, err
	}
	hash := sha256.New()
	identity := TreeIdentity{}
	entries := map[string]treeEntryIdentity{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("identity path escapes tree")
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported identity entry %q", filepath.ToSlash(rel))
		}
		if info.IsDir() {
			return nil
		}
		if info.Sys() != nil && linkCount(info) > 1 {
			return fmt.Errorf("hard-linked identity entry %q is not allowed", filepath.ToSlash(rel))
		}
		identity.Files++
		identity.Bytes += info.Size()
		if identity.Files > limits.MaxFiles || identity.Bytes > limits.MaxBytes || info.Size() > limits.MaxFileSize {
			return fmt.Errorf("identity tree exceeds declared limits")
		}
		data, err := readRegularFile(path, info.Size(), limits.MaxFileSize)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fileHash := sha256.Sum256(data)
		entries[rel] = treeEntryIdentity{digest: hex.EncodeToString(fileHash[:]), mode: info.Mode().Perm()}
		_, _ = io.WriteString(hash, rel)
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return TreeIdentity{}, nil, err
	}
	identity.Digest = hex.EncodeToString(hash.Sum(nil))
	return identity, entries, nil
}

func readRegularFile(path string, expected, max int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != expected || int64(len(data)) > max {
		return nil, fmt.Errorf("isolation source changed during copy")
	}
	return data, nil
}

func SortedEnvironment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "" && !strings.ContainsAny(key, "=\x00") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if !strings.ContainsRune(values[key], '\x00') {
			out = append(out, key+"="+values[key])
		}
	}
	return out
}
