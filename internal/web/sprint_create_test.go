package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

var errTestCreationFailed = errors.New("workspace root unavailable")

func TestSprintCreateCreatesWorkspaceAndRedirectsToSprintPage(t *testing.T) {
	fake := sampleQueries()
	h := testHandler(t, fake, nil)
	cookie, csrf := establishOperationSession(t, h)

	res := operationFormRequest(h, "/projects/alpha/sprints/create", url.Values{"_csrf": {csrf}, "sprint": {"31-web-operations"}}, cookie)
	if res.Code != http.StatusSeeOther || res.Header().Get("Location") != "/projects/alpha/sprints/31-web-operations" {
		t.Fatalf("status=%d headers=%v", res.Code, res.Header())
	}
	if fake.createdProject != "alpha" || fake.createdSprint != "31-web-operations" {
		t.Fatalf("created project=%q sprint=%q", fake.createdProject, fake.createdSprint)
	}

	rejected := operationFormRequest(h, "/projects/alpha/sprints/create", url.Values{"_csrf": {csrf}, "sprint": {"../escape"}}, cookie)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("unsafe slug status=%d body=%s", rejected.Code, rejected.Body.String())
	}
	if fake.createdSprint == "../escape" {
		t.Fatal("unsafe slug reached workspace creation")
	}

	forged := operationFormRequest(h, "/projects/alpha/sprints/create", url.Values{"_csrf": {"wrong"}, "sprint": {"31-web-operations"}}, cookie)
	if forged.Code != http.StatusForbidden {
		t.Fatalf("csrf status=%d", forged.Code)
	}

	get := operationSessionRequest(h, http.MethodGet, "/projects/alpha/sprints/create", cookie)
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("get status=%d", get.Code)
	}
}

func TestSprintCreateSurfacesCreationFailure(t *testing.T) {
	fake := sampleQueries()
	fake.createErr = errTestCreationFailed
	h := testHandler(t, fake, nil)
	cookie, csrf := establishOperationSession(t, h)

	res := operationFormRequest(h, "/projects/alpha/sprints/create", url.Values{"_csrf": {csrf}, "sprint": {"31-web-operations"}}, cookie)
	if res.Code != http.StatusUnprocessableEntity || !strings.Contains(res.Body.String(), "Creation failed") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
