package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type ProjectReasoningLock struct {
	SchemaVersion int                            `json:"schema_version"`
	Project       string                         `json:"project"`
	Sprint        string                         `json:"sprint"`
	CapturedAt    time.Time                      `json:"captured_at"`
	Documents     []ProjectReasoningLockDocument `json:"documents"`
}
type ProjectReasoningLockDocument struct{ Name, Path, SHA256 string }

func (s Service) writeProjectReasoningLock(sp Sprint, index SprintIndex) error {
	if len(index.ProjectReasoning) == 0 {
		return nil
	}
	lock := ProjectReasoningLock{SchemaVersion: 1, Project: sp.Project, Sprint: sp.Slug, CapturedAt: s.now().UTC()}
	for _, item := range index.ProjectReasoning {
		full, err := workspace.ResolveInside(s.root, normalizeWorkspacePath(item.Path))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lock.Documents = append(lock.Documents, ProjectReasoningLockDocument{Name: item.Name, Path: item.Path, SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(lock.Documents, func(i, j int) bool { return lock.Documents[i].Path < lock.Documents[j].Path })
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(sp.Path, "project-reasoning-lock.json")
	tmp := path + ".candidate"
	if err = os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write project reasoning lock candidate: %w", err)
	}
	return os.Rename(tmp, path)
}
