package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Antonio7098/ultraplan-go/internal/productstate"
	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
	"github.com/Antonio7098/ultraplan-go/internal/study"
)

type storageMigrationItem struct {
	Kind   string `json:"kind"`
	Scope  string `json:"scope"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type storageMigrationResult struct {
	SchemaVersion int                    `json:"schema_version"`
	DryRun        bool                   `json:"dry_run"`
	Imported      int                    `json:"imported"`
	Skipped       int                    `json:"skipped"`
	Failed        int                    `json:"failed"`
	Items         []storageMigrationItem `json:"items"`
}

func runStorage(deps dependencies, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := deps.stdout.Write([]byte(storageHelp()))
		return err
	}
	if args[0] != "migrate" {
		return classified(ExitUsage, "storage: expected 'migrate'")
	}
	dryRun, jsonOut := false, false
	for _, arg := range args[1:] {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--json":
			jsonOut = true
		case "--help", "-h":
			_, err := deps.stdout.Write([]byte(storageHelp()))
			return err
		default:
			return classified(ExitUsage, "storage migrate: unknown argument %q", arg)
		}
	}
	root, err := discoverWorkspace(deps)
	if err != nil {
		return err
	}
	if !dryRun {
		repository, err := runcontrol.OpenSQLite(deps.ctx, root.Path, runcontrol.SQLiteOptions{})
		if err != nil {
			return classifiedCause(ExitWorkspace, err, "storage.migrate: open workspace database")
		}
		defer repository.Close()
		if _, err := productstate.Ensure(root.Path); err != nil {
			return classifiedCause(ExitWorkspace, err, "storage.migrate: create product-state schema")
		}
	}
	result := storageMigrationResult{SchemaVersion: 1, DryRun: dryRun, Items: []storageMigrationItem{}}
	appendItem := func(item storageMigrationItem) {
		result.Items = append(result.Items, item)
		switch item.Status {
		case "imported", "would_import":
			result.Imported++
		case "skipped":
			result.Skipped++
		case "failed":
			result.Failed++
		}
	}
	studies, err := study.DiscoverStudies(root.Path)
	if err != nil {
		return classifiedCause(ExitWorkspace, err, "storage.migrate: discover studies")
	}
	for _, item := range studies {
		path := study.RunStatePath(item)
		if _, err := os.Stat(path); err != nil {
			if !os.IsNotExist(err) {
				appendItem(storageMigrationItem{Kind: "study_run", Scope: item.Name, Path: path, Status: "failed", Error: err.Error()})
			}
			continue
		}
		stored, err := study.RunStateInDatabase(item)
		if err != nil {
			appendItem(storageMigrationItem{Kind: "study_run", Scope: item.Name, Path: path, Status: "failed", Error: err.Error()})
			continue
		}
		if stored {
			appendItem(storageMigrationItem{Kind: "study_run", Scope: item.Name, Path: path, Status: "skipped"})
			continue
		}
		state, err := study.LoadRunState(item)
		if err == nil && !dryRun {
			err = study.MigrateRunStateToDatabase(item, state)
		}
		appendItem(migrationItem("study_run", item.Name, path, dryRun, err))
	}
	projects, err := project.DiscoverProjects(root.Path)
	if err != nil {
		return classifiedCause(ExitWorkspace, err, "storage.migrate: discover projects")
	}
	for _, projectItem := range projects {
		sprints, err := sprint.DiscoverSprints(root.Path, projectItem)
		if err != nil {
			appendItem(storageMigrationItem{Kind: "project", Scope: projectItem.Name, Path: projectItem.Path, Status: "failed", Error: err.Error()})
			continue
		}
		for _, sprintItem := range sprints {
			migrateSprintState(root.Path, sprintItem, dryRun, appendItem)
		}
	}
	if jsonOut {
		if err := json.NewEncoder(deps.stdout).Encode(result); err != nil {
			return err
		}
	} else {
		for _, item := range result.Items {
			fmt.Fprintf(deps.stdout, "%-12s %-14s %s %s", item.Status, item.Kind, item.Scope, filepath.ToSlash(item.Path))
			if item.Error != "" {
				fmt.Fprintf(deps.stdout, " | %s", item.Error)
			}
			fmt.Fprintln(deps.stdout)
		}
		fmt.Fprintf(deps.stdout, "Product-state migration: %d imported, %d skipped, %d failed\n", result.Imported, result.Skipped, result.Failed)
	}
	if result.Failed > 0 {
		return classified(ExitPartial, "storage.migrate: %d state artifact(s) failed validation or import", result.Failed)
	}
	return nil
}

func migrateSprintState(root string, sp sprint.Sprint, dryRun bool, appendItem func(storageMigrationItem)) {
	scope := sp.Project + "/" + sp.Slug
	flowPath, flowPathErr := sprint.FlowStatePath(root, sp)
	if flowPathErr == nil {
		if _, err := os.Stat(flowPath); err == nil {
			stored, checkErr := sprint.FlowStateInDatabase(root, sp)
			if checkErr != nil {
				appendItem(storageMigrationItem{Kind: "sprint_flow", Scope: scope, Path: flowPath, Status: "failed", Error: checkErr.Error()})
			} else if stored {
				appendItem(storageMigrationItem{Kind: "sprint_flow", Scope: scope, Path: flowPath, Status: "skipped"})
			} else {
				state, loadErr := sprint.LoadFlowState(root, sp)
				if loadErr == nil && !dryRun {
					loadErr = sprint.MigrateFlowStateToDatabase(root, sp, state)
				}
				appendItem(migrationItem("sprint_flow", scope, flowPath, dryRun, loadErr))
			}
		} else if !os.IsNotExist(err) {
			appendItem(storageMigrationItem{Kind: "sprint_flow", Scope: scope, Path: flowPath, Status: "failed", Error: err.Error()})
		}
	}
	executePath, executePathErr := sprint.ExecuteRunStatePath(root, sp)
	if executePathErr == nil {
		if _, err := os.Stat(executePath); err == nil {
			stored, checkErr := sprint.ExecuteStateInDatabase(root, sp)
			if checkErr != nil {
				appendItem(storageMigrationItem{Kind: "sprint_execute", Scope: scope, Path: executePath, Status: "failed", Error: checkErr.Error()})
			} else if stored {
				appendItem(storageMigrationItem{Kind: "sprint_execute", Scope: scope, Path: executePath, Status: "skipped"})
			} else {
				state, loadErr := sprint.LoadExecuteRunState(root, sp)
				if loadErr == nil && !dryRun {
					loadErr = sprint.MigrateExecuteStateToDatabase(root, sp, state)
				}
				appendItem(migrationItem("sprint_execute", scope, executePath, dryRun, loadErr))
			}
		} else if !os.IsNotExist(err) {
			appendItem(storageMigrationItem{Kind: "sprint_execute", Scope: scope, Path: executePath, Status: "failed", Error: err.Error()})
		}
	}
}

func migrationItem(kind, scope, path string, dryRun bool, err error) storageMigrationItem {
	if err != nil {
		return storageMigrationItem{Kind: kind, Scope: scope, Path: path, Status: "failed", Error: err.Error()}
	}
	status := "imported"
	if dryRun {
		status = "would_import"
	}
	return storageMigrationItem{Kind: kind, Scope: scope, Path: path, Status: status}
}

func storageHelp() string {
	return `ultraplan storage migrate

Usage:
  ultraplan [--workspace <path>] storage migrate [--dry-run] [--json]

Imports existing study run state, sprint flow state, and sprint execute state
into the workspace SQLite database. Existing files remain as checkpoints.
`
}
