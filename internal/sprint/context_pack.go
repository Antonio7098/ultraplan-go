package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const contextPackSchemaVersion = 1

// sprintContextPack is a derived, disposable cache. requirements.md and
// code-context.md remain authoritative; freezing resolved source bytes keeps
// the shared foundation stable after execute changes live line numbers.
type sprintContextPack struct {
	SchemaVersion      int       `json:"schema_version"`
	Project            string    `json:"project"`
	Sprint             string    `json:"sprint"`
	Key                string    `json:"key"`
	RequirementsDigest string    `json:"requirements_sha256"`
	CodeContextDigest  string    `json:"code_context_sha256"`
	TargetDigest       string    `json:"target_sha256"`
	PrefixDigest       string    `json:"prefix_sha256"`
	Prefix             string    `json:"prefix"`
	CreatedAt          time.Time `json:"created_at"`
}

func contextPackIdentity(requirements, codeContext, target string) (key, requirementsDigest, codeContextDigest, targetDigest string) {
	requirementsDigest = digestString(requirements)
	codeContextDigest = digestString(codeContext)
	targetDigest = digestString(filepath.Clean(target))
	key = digestString("sprint-context-v1\x00" + requirementsDigest + "\x00" + codeContextDigest + "\x00" + targetDigest)
	return
}

func digestString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func contextPackPath(root string, sp Sprint, key string) string {
	return filepath.Join(root, ".ultra", "cache", "sprint-context", sp.Project, sp.Slug, key+".json")
}

func loadContextPack(root string, sp Sprint, requirements, codeContext, target string) (string, error) {
	key, requirementsDigest, codeContextDigest, targetDigest := contextPackIdentity(requirements, codeContext, target)
	data, err := os.ReadFile(contextPackPath(root, sp, key))
	if err != nil {
		return "", err
	}
	var pack sprintContextPack
	if err := json.Unmarshal(data, &pack); err != nil {
		return "", err
	}
	if pack.SchemaVersion != contextPackSchemaVersion || pack.Project != sp.Project || pack.Sprint != sp.Slug || pack.Key != key || pack.RequirementsDigest != requirementsDigest || pack.CodeContextDigest != codeContextDigest || pack.TargetDigest != targetDigest {
		return "", errors.New("context pack identity mismatch")
	}
	if len(pack.Prefix) > maxSharedPromptPrefixBytes || !strings.HasSuffix(pack.Prefix, sharedPromptStageBoundary) || digestString(pack.Prefix) != pack.PrefixDigest {
		return "", errors.New("context pack payload is invalid")
	}
	return pack.Prefix, nil
}

func saveContextPack(root string, sp Sprint, requirements, codeContext, target, prefix string, now time.Time) error {
	if len(prefix) > maxSharedPromptPrefixBytes || !strings.HasSuffix(prefix, sharedPromptStageBoundary) {
		return fmt.Errorf("refuse invalid shared context pack")
	}
	key, requirementsDigest, codeContextDigest, targetDigest := contextPackIdentity(requirements, codeContext, target)
	pack := sprintContextPack{
		SchemaVersion: contextPackSchemaVersion, Project: sp.Project, Sprint: sp.Slug, Key: key,
		RequirementsDigest: requirementsDigest, CodeContextDigest: codeContextDigest, TargetDigest: targetDigest,
		PrefixDigest: digestString(prefix), Prefix: prefix, CreatedAt: now.UTC(),
	}
	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}
	path := contextPackPath(root, sp, key)
	if err := atomicWriteFile(path, append(data, '\n')); err != nil {
		return err
	}
	return pruneContextPacks(filepath.Dir(path), 8)
}

func pruneContextPacks(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type candidate struct {
		path    string
		modTime time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		candidates = append(candidates, candidate{path: filepath.Join(dir, entry.Name()), modTime: info.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].modTime.After(candidates[j].modTime) })
	if len(candidates) <= keep {
		return nil
	}
	for _, old := range candidates[keep:] {
		if err := os.Remove(old.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
