package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestIntegrationStudyAndSprintSurfaceAgreement(t *testing.T) {
	root := t.TempDir()
	var stderr bytes.Buffer
	if status := app.Run(app.Config{Args: []string{"init-workspace", "--path", root}, Stderr: &stderr}); status != app.ExitOK {
		t.Fatalf("init status=%d stderr=%s", status, stderr.String())
	}
	writeIntegrationFile(t, root, "projects/alpha/project-index.md", "# Project Index\n\n## Project Scope\n\n- Target Implementation Directory: /tmp/alpha\n")
	writeIntegrationFile(t, root, "projects/alpha/roadmap.md", "# Roadmap\n")
	writeIntegrationFile(t, root, "projects/alpha/docs/PRD.md", "# Product\n")
	for _, artifact := range []string{"requirements.md", "sprint-index.md", "technical-handbook.md", "reasoning.md", "plan.md"} {
		writeIntegrationFile(t, root, "projects/alpha/sprints/32-hardening/"+artifact, "# "+artifact+"\n")
	}
	writeIntegrationFile(t, root, "studies/research/sources/source.md", "# Source\n")
	writeIntegrationFile(t, root, "studies/research/dimensions/01-contract.md", "# Contract\n")

	useCases := app.NewWebUseCases(root, app.WebUseCaseOptions{})
	operations, ok := useCases.(app.WebOperations)
	if !ok {
		t.Fatal("shared app use cases do not expose operations")
	}
	h, err := NewHandler(HandlerOptions{
		Queries: useCases, Operations: operations, Authority: testAuthority,
		Now:       func() time.Time { return time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC) },
		RequestID: func() string { return "integration-id" },
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		html, api, marker string
	}{
		{"/projects/alpha/sprints/32-hardening", "/api/v1/projects/alpha/sprints/32-hardening", "32-hardening"},
		{"/studies/research", "/api/v1/studies/research", "research"},
	} {
		html := request(h, http.MethodGet, tc.html, nil)
		api := request(h, http.MethodGet, tc.api, nil)
		if html.Code != http.StatusOK || api.Code != http.StatusOK || !strings.Contains(html.Body.String(), tc.marker) || !strings.Contains(api.Body.String(), tc.marker) {
			t.Fatalf("surface disagreement for %s: html=%d api=%d\nhtml=%s\napi=%s", tc.marker, html.Code, api.Code, html.Body.String(), api.Body.String())
		}
	}

	cookie, csrf := establishOperationSession(t, h)
	prepareBody := `{"operation":{"kind":"validation","scope":{"project":"alpha"}}}`
	prepared := operationMutationRequest(h, http.MethodPost, "/api/v1/operations/prepare", prepareBody, cookie, csrf)
	if prepared.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", prepared.Code, prepared.Body.String())
	}
	var preparation struct {
		Data preparationDTO `json:"data"`
	}
	if err := json.Unmarshal(prepared.Body.Bytes(), &preparation); err != nil {
		t.Fatal(err)
	}
	startBody := `{"operation":{"kind":"validation","scope":{"project":"alpha"}},"confirmation_token":"` + preparation.Data.ConfirmationToken + `"}`
	started := operationMutationRequest(h, http.MethodPost, "/api/v1/operations", startBody, cookie, csrf)
	if started.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", started.Code, started.Body.String())
	}
	var operation struct {
		Data operationDocument `json:"data"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &operation); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !terminalOperationState(operation.Data.State) && time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operation.Data.ID, nil)
		req.Host = testAuthority
		req.AddCookie(cookie)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if err := json.Unmarshal(res.Body.Bytes(), &operation); err != nil {
			t.Fatal(err)
		}
	}
	if operation.Data.State != "succeeded" || operation.Data.Result == nil {
		t.Fatalf("terminal operation=%+v", operation.Data)
	}
}

func writeIntegrationFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
