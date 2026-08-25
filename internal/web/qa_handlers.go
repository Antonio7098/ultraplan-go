package web

import (
	"net/http"

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
	model := pageModel{Title: "Read-only QA · " + sprintResult.Slug, Heading: sprintResult.Slug, Sprint: &sprintResult, QA: &qa, Page: "qa"}
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
