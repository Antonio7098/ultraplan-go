package sprint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func qaLegacyInvestigatorWorkspacePath(root, attemptID, shardID string) string {
	return filepath.Join(os.TempDir(), "ultraplan-qa-investigators", hashOpaque(filepath.Clean(root))[:24], attemptID, shardID)
}

// OpenCode retains the original directory in its session store. A CLI workdir
// change alone does not relocate it. Expose only the verified managed copy at
// that old name while continuing the session; no source data is copied to /tmp.
func qaLegacyWorkspaceAlias(legacy, workspace string) (func(), error) {
	rel, err := filepath.Rel(os.TempDir(), legacy)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !strings.HasPrefix(rel, "ultraplan-qa-investigators"+string(filepath.Separator)) {
		return nil, errors.New("invalid legacy investigator workspace path")
	}
	if legacy == workspace {
		return func() {}, nil
	}
	if info, err := os.Lstat(workspace); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("managed investigator workspace is unavailable or unsafe")
	}
	root, err := os.OpenRoot(os.TempDir())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent := ""
	for _, part := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		parent = filepath.Join(parent, part)
		if err := root.Mkdir(parent, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, err := root.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("legacy investigator parent is unsafe: %s", parent)
		}
	}
	if err := root.Symlink(workspace, rel); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if link, err := root.Readlink(rel); err != nil || link != workspace {
			return nil, errors.New("legacy investigator path is occupied; refusing to replace it")
		}
	}
	return func() {
		// Never remove a directory or a link that another owner replaced.
		if link, err := os.Readlink(legacy); err == nil && link == workspace {
			_ = os.Remove(legacy)
		}
	}, nil
}
