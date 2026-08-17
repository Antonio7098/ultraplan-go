package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestArtifactPreviewContractAndHostileSourceEscaping(t *testing.T) {
	queries := sampleQueries()
	hostile := `<script>alert("x")</script><img src=x onerror=alert(1)>`
	queries.artifact = app.WebArtifactPreview{
		Ref: "artifact_ref", DisplayPath: "projects/alpha/plan.md", MediaType: "text/markdown",
		Content: hostile, SizeBytes: int64(len(hostile)), ReturnedBytes: len(hostile),
	}
	h := testHandler(t, queries, nil)
	html := request(h, http.MethodGet, "/artifacts/artifact_ref", nil)
	if html.Code != http.StatusOK || strings.Contains(html.Body.String(), hostile) ||
		strings.Contains(html.Body.String(), `<script>alert`) || strings.Contains(html.Body.String(), "onerror=") {
		t.Fatalf("unsafe artifact HTML: %s", html.Body.String())
	}
	api := request(h, http.MethodGet, "/api/v1/artifacts/artifact_ref", nil)
	if api.Code != http.StatusOK || !strings.Contains(api.Body.String(), `\u003cscript\u003e`) ||
		api.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("artifact API status=%d headers=%v body=%s", api.Code, api.Header(), api.Body.String())
	}
}

func TestMarkdownArtifactRendersAsDocument(t *testing.T) {
	queries := sampleQueries()
	queries.artifact.Content = "# Heading\n\n- one\n- two\n\n| A | B |\n| - | - |\n| x | y |\n"
	queries.artifact.SizeBytes = int64(len(queries.artifact.Content))
	queries.artifact.ReturnedBytes = len(queries.artifact.Content)
	body := request(testHandler(t, queries, nil), http.MethodGet, "/artifacts/artifact_ref", nil).Body.String()
	for _, want := range []string{`class="markdown-body"`, "<h1>Heading</h1>", "<li>one</li>", "<table>"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered Markdown missing %q: %s", want, body)
		}
	}
}

func TestArtifactInvalidAppContractFailsSafely(t *testing.T) {
	queries := sampleQueries()
	queries.artifact.ReturnedBytes = app.WebPreviewByteLimit + 1
	queries.artifact.SizeBytes = int64(queries.artifact.ReturnedBytes)
	h := testHandler(t, queries, nil)
	api := request(h, http.MethodGet, "/api/v1/artifacts/artifact_ref", nil)
	if api.Code != http.StatusInternalServerError || !strings.Contains(api.Body.String(), `"code":"internal_error"`) ||
		strings.Contains(api.Body.String(), "invalid artifact") {
		t.Fatalf("status=%d body=%s", api.Code, api.Body.String())
	}
}
