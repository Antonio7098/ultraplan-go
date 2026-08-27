package web

import (
	"net/http"
	"strconv"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func (h *handler) handleSprintQA(w http.ResponseWriter, r *http.Request, project, sprint string) {
	if h.qa == nil {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := h.qa.QAStatus(r.Context(), app.QARequest{Project: project, Sprint: sprint})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAEvidence(w http.ResponseWriter, r *http.Request, project, sprintSlug, evidence string) {
	queries, ok := h.qa.(app.QAEvidenceQueries)
	if !ok {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := queries.QAEvidence(r.Context(), app.QARequest{Project: project, Sprint: sprintSlug, Evidence: evidence})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAAdjudication(w http.ResponseWriter, r *http.Request, project, sprintSlug string) {
	queries, ok := h.qa.(app.QAEvidenceQueries)
	if !ok {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := queries.QAAdjudication(r.Context(), app.QARequest{Project: project, Sprint: sprintSlug})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAIssues(w http.ResponseWriter, r *http.Request, project, sprintSlug string) {
	queries, ok := h.qa.(app.QAEvidenceQueries)
	if !ok {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	limit := 0
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			h.handleQueryError(w, r, true, err)
			return
		}
		limit = parsed
	}
	result, err := queries.QAIssues(r.Context(), app.QARequest{Project: project, Sprint: sprintSlug, Cursor: r.URL.Query().Get("cursor"), Limit: limit})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAIssue(w http.ResponseWriter, r *http.Request, project, sprintSlug, issue string) {
	queries, ok := h.qa.(app.QAEvidenceQueries)
	if !ok {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := queries.QAIssue(r.Context(), app.QARequest{Project: project, Sprint: sprintSlug, Issue: issue})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAAssessment(w http.ResponseWriter, r *http.Request, project, sprintSlug string) {
	queries, ok := h.qa.(app.QAEvidenceQueries)
	if !ok {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := queries.QAAssessment(r.Context(), app.QARequest{Project: project, Sprint: sprintSlug})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQASmokeSuite(w http.ResponseWriter, r *http.Request, project, sprintSlug string) {
	queries, ok := h.qa.(app.QAEvidenceQueries)
	if !ok {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := queries.QASmokeSuite(r.Context(), app.QARequest{Project: project, Sprint: sprintSlug})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAMap(w http.ResponseWriter, r *http.Request, project, sprint string) {
	if h.qa == nil {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := h.qa.QAMap(r.Context(), app.QARequest{Project: project, Sprint: sprint})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAShard(w http.ResponseWriter, r *http.Request, project, sprint, shard string) {
	if h.qa == nil {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := h.qa.QAShard(r.Context(), app.QARequest{Project: project, Sprint: sprint, Shard: shard})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQATheory(w http.ResponseWriter, r *http.Request, project, sprint, theory string) {
	if h.qa == nil {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := h.qa.QATheory(r.Context(), app.QARequest{Project: project, Sprint: sprint, Theory: theory})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQASynthesis(w http.ResponseWriter, r *http.Request, project, sprint string) {
	if h.qa == nil {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := h.qa.QASynthesis(r.Context(), app.QARequest{Project: project, Sprint: sprint})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintQAPage(w http.ResponseWriter, r *http.Request, project, sprint, shard, theory string) {
	if h.qa == nil {
		h.handleQueryError(w, r, false, app.ErrWebUnavailable)
		return
	}
	sprintResult, err := h.queries.Sprint(r.Context(), project, sprint)
	if err != nil {
		h.handleQueryError(w, r, false, err)
		return
	}
	qa, err := h.qa.QAStatus(r.Context(), app.QARequest{Project: project, Sprint: sprint})
	if err != nil {
		h.handleQueryError(w, r, false, err)
		return
	}
	model := pageModel{Title: "QA · " + sprintResult.Slug, Heading: sprintResult.Slug, Sprint: &sprintResult, QA: &qa, Page: "qa"}
	if shard != "" {
		focused, focusErr := h.qa.QAShard(r.Context(), app.QARequest{Project: project, Sprint: sprint, Shard: shard})
		if focusErr != nil {
			h.handleQueryError(w, r, false, focusErr)
			return
		}
		model.QAShard = &focused
	}
	if theory != "" {
		focused, focusErr := h.qa.QATheory(r.Context(), app.QARequest{Project: project, Sprint: sprint, Theory: theory})
		if focusErr != nil {
			h.handleQueryError(w, r, false, focusErr)
			return
		}
		model.QATheory = &focused
	}
	h.render(w, r, http.StatusOK, "sprint", model)
}

func (h *handler) handleSprintRepair(w http.ResponseWriter, r *http.Request, project, sprintSlug, resource string) {
	if h.repair == nil {
		h.handleQueryError(w, r, true, app.ErrWebUnavailable)
		return
	}
	result, err := h.repair.RepairStatus(r.Context(), app.RepairRequest{Project: project, Sprint: sprintSlug, RepairRunID: r.URL.Query().Get("run")})
	if err != nil {
		h.handleQueryError(w, r, true, err)
		return
	}
	missing := resource != "api_sprint_repair" && result.Packet == nil
	if missing {
		h.handleQueryError(w, r, true, app.ErrWebNotFound)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result, nil)
}

func (h *handler) handleSprintRepairPage(w http.ResponseWriter, r *http.Request, project, sprintSlug string) {
	if h.repair == nil {
		h.handleQueryError(w, r, false, app.ErrWebUnavailable)
		return
	}
	sprintResult, err := h.queries.Sprint(r.Context(), project, sprintSlug)
	if err != nil {
		h.handleQueryError(w, r, false, err)
		return
	}
	repair, err := h.repair.RepairStatus(r.Context(), app.RepairRequest{Project: project, Sprint: sprintSlug})
	if err != nil {
		h.handleQueryError(w, r, false, err)
		return
	}
	h.render(w, r, http.StatusOK, "sprint", pageModel{Title: "Repair · " + sprintResult.Slug, Heading: sprintResult.Slug, Sprint: &sprintResult, Repair: &repair, Page: "repair"})
}
