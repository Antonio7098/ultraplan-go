package sprint

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type executionHandoff struct {
	SchemaVersion   int                    `json:"schemaVersion"`
	PlanFingerprint string                 `json:"planFingerprint,omitempty"`
	Status          string                 `json:"status,omitempty"`
	ChangedPaths    []string               `json:"changedPaths,omitempty"`
	Tasks           []executionHandoffTask `json:"tasks,omitempty"`
}

type executionHandoffTask struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Status      ExecuteTaskStatus   `json:"status"`
	Attempts    int                 `json:"attempts"`
	Evidence    []ExecuteEvidence   `json:"evidence,omitempty"`
	Diagnostics []ExecuteDiagnostic `json:"diagnostics,omitempty"`
}

func executionHandoffContent(data []byte, changedPaths []string) (string, error) {
	handoff := executionHandoff{SchemaVersion: 1, ChangedPaths: uniqueSorted(changedPaths)}
	var state ExecuteRunState
	if err := json.Unmarshal(data, &state); err == nil && state.SchemaVersion == ExecuteRunStateSchemaVersion {
		handoff.PlanFingerprint = state.PlanFingerprint
		for _, task := range state.Tasks {
			item := executionHandoffTask{ID: task.ID, Name: task.Identity.Name, Status: task.Status, Attempts: task.Attempts, Diagnostics: task.Diagnostics}
			for _, evidence := range task.Evidence {
				if evidence.Kind != "changed-path" {
					item.Evidence = append(item.Evidence, evidence)
				}
			}
			handoff.Tasks = append(handoff.Tasks, item)
		}
	} else {
		var legacy struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return "", err
		}
		handoff.Status = legacy.Status
	}
	encoded, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(encoded, '\n')), nil
}

func executionHandoffPath(sp Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, ".execution-handoff.json"))
}

func directExecutionHandoffInput(root string, sp Sprint) directPromptInput {
	path := ExecuteRunStateRelPath(sp)
	full, err := workspace.ResolveInside(root, path)
	if err != nil {
		return directPromptInput{ID: "execution-handoff", Kind: "execution", Path: executionHandoffPath(sp), Missing: directInputReadError(root, err)}
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return directPromptInput{ID: "execution-handoff", Kind: "execution", Path: executionHandoffPath(sp), Missing: directInputReadError(root, err)}
	}
	content, err := executionHandoffContent(data, reviewChangedPaths(data))
	if err != nil {
		return directPromptInput{ID: "execution-handoff", Kind: "execution", Path: executionHandoffPath(sp), Missing: "run state could not be summarized"}
	}
	return directContentInput("execution-handoff", "execution", executionHandoffPath(sp), content)
}
