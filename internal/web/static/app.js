(() => {
  "use strict";

  const refreshStatus = document.getElementById("refresh-status");
  for (const link of document.querySelectorAll(".refresh-link")) {
    link.addEventListener("click", () => {
      if (refreshStatus) refreshStatus.textContent = "Refreshing current workspace state.";
    });
  }

  for (const stack of document.querySelectorAll("[data-sidebar-stack]")) {
    const showPanel = (id) => {
      const target = stack.querySelector(`#${CSS.escape(id)}`);
      if (!target) return false;
      for (const panel of stack.querySelectorAll("[data-sidebar-panel]")) panel.hidden = panel !== target;
      target.querySelector("a, button")?.focus();
      return true;
    };
    stack.addEventListener("click", (event) => {
      const back = event.target.closest?.("[data-sidebar-back]");
      if (back) {
        event.preventDefault();
        showPanel(back.dataset.sidebarBack);
        return;
      }
      const open = event.target.closest?.("[data-sidebar-open]");
      if (open && showPanel(open.dataset.sidebarOpen)) event.preventDefault();
    });
  }

  for (const details of document.querySelectorAll(".detail-sidebar details")) {
    let pinnedOpen = details.open;
    details.addEventListener("pointerenter", (event) => {
      if (event.pointerType === "touch") return;
      details.open = true;
    });
    details.addEventListener("pointerleave", (event) => {
      if (event.pointerType === "touch") return;
      details.open = pinnedOpen;
    });
    details.querySelector(":scope > summary")?.addEventListener("click", (event) => {
      event.preventDefault();
      pinnedOpen = !pinnedOpen;
      details.open = pinnedOpen;
    });
  }

  for (const workspace of document.querySelectorAll("[data-stage-workspace]")) {
    const buttons = [...workspace.querySelectorAll("[data-stage-select]")];
    const panels = [...workspace.querySelectorAll("[data-stage-panel]")];
    const selectStage = (id, moveFocus = false) => {
      const panel = panels.find((item) => item.id === id);
      if (!panel) return;
      for (const item of panels) item.hidden = item !== panel;
      for (const button of buttons) {
        const selected = button.dataset.stageSelect === id;
        button.setAttribute("aria-selected", String(selected));
        button.tabIndex = selected ? 0 : -1;
      }
      if (moveFocus) panel.focus();
      history.replaceState(null, "", `#${id}`);
    };
    for (const button of buttons) button.addEventListener("click", () => selectStage(button.dataset.stageSelect, true));
    const requested = location.hash.slice(1);
    const initial = panels.some((item) => item.id === requested)
      ? requested
      : buttons.find((button) => button.closest(".stage-running"))?.dataset.stageSelect
        || buttons.find((button) => !button.closest(".stage-complete, .stage-completed, .stage-skipped"))?.dataset.stageSelect
        || buttons[buttons.length - 1]?.dataset.stageSelect;
    if (initial) selectStage(initial);
  }

  const forms = [...document.querySelectorAll(".operation-form")];
  const statusRoot = document.querySelector("[data-operation-id]");
  const reviewStatus = document.querySelector("[data-review-status]");
  const reviewerDialog = document.getElementById("reviewer-result-dialog");
  const reviewerDialogContent = document.getElementById("reviewer-result-content");
  const reviewerDialogClose = document.getElementById("reviewer-result-close");
  if (forms.length === 0 && !statusRoot && !reviewStatus && !document.querySelector(".reviewer-card")) return;

  const csrf = document.querySelector('meta[name="ultraplan-csrf"]')?.content || "";
  let live = document.getElementById("operation-live");
  let timeline = document.getElementById("operation-timeline");
  let cancelButton = document.getElementById("operation-cancel");
  const reviewerGrid = document.getElementById("live-reviewer-grid");
  const reviewerEmpty = document.getElementById("reviewer-grid-empty");
  let stream = null;
  let reviewTimer = null;
  let reviewRefreshActive = false;
  let currentOperationID = "";
  let lastSequence = 0;

  document.addEventListener("click", (event) => {
    const trigger = event.target.closest?.(".reviewer-result-open");
    if (!trigger || !reviewerDialog || !reviewerDialogContent) return;
    const fullResult = trigger.parentElement?.querySelector(".reviewer-full-result")?.textContent || "";
    reviewerDialogContent.textContent = fullResult;
    reviewerDialog.showModal();
  });
  reviewerDialogClose?.addEventListener("click", () => reviewerDialog.close());
  reviewerDialog?.addEventListener("click", (event) => {
    if (event.target === reviewerDialog) reviewerDialog.close();
  });

  function specification(form, submitter) {
    const scope = {};
    if (form.dataset.project) scope.project = form.dataset.project;
    if (form.dataset.sprint) scope.sprint = form.dataset.sprint;
    if (form.dataset.study) scope.study = form.dataset.study;
    const options = {};
    const selectedStage = form.elements?.stage?.value || form.dataset.stage;
    if (selectedStage) options.to_stage = selectedStage;
    if (form.dataset.parallelism) options.parallelism = Number(form.dataset.parallelism);
    return {kind: submitter?.dataset.operationKind || form.dataset.operationKind, scope, options};
  }

  async function command(path, payload, method = "POST") {
    const response = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf},
      body: payload === null ? undefined : JSON.stringify(payload)
    });
    const body = await response.json();
    if (!response.ok) {
      const parts = [body.error?.message, body.error?.details?.reason, body.error?.details?.guidance].filter(Boolean);
      throw new Error(parts.join(" ") || `Request failed (${response.status})`);
    }
    return body.data;
  }

  function announce(message, isError = false) {
    if (!live) return;
    live.textContent = message;
    live.classList.toggle("operation-error", isError);
    if (isError) live.focus?.();
  }

  function appendEvent(name, event) {
    const sequence = Number(event.sequence || 0);
    if (sequence && sequence <= lastSequence) return;
    if (sequence) lastSequence = sequence;
    if (!timeline) return;
    const item = document.createElement("li");
    const payload = event.payload || {};
    const context = [payload.stage, payload.task].filter(Boolean).join(" · ");
    const progress = payload.total > 0 ? ` (${payload.completed || 0}/${payload.total})` : "";
    item.textContent = `${context ? `[${context}] ` : ""}${payload.message || payload.state || payload.reason || name}${progress}`;
    item.dataset.event = name;
    timeline.append(item);
    while (timeline.children.length > 100) timeline.firstElementChild.remove();
    timeline.scrollTop = timeline.scrollHeight;
  }

  function reviewerStatusClass(status) {
    if (status === "completed") return "ok";
    if (status === "running") return "info";
    if (status === "pending") return "warn";
    return "error";
  }

  function setReviewCount(id, value) {
    const node = document.getElementById(id);
    if (node) node.textContent = String(value || 0);
  }

  async function refreshReviewers() {
    if (!reviewStatus || !reviewerGrid || reviewRefreshActive) return;
    const path = reviewStatus.dataset.reviewStatusPath;
    if (!path) return;
    reviewRefreshActive = true;
    try {
      const response = await fetch(path, {credentials: "same-origin"});
      const body = await response.json();
      if (!response.ok) throw new Error(body.error?.message || `Reviewer status failed (${response.status})`);
      const review = body.data?.review || {};
      const reviewers = Array.isArray(review.reviewers) ? review.reviewers : [];
      setReviewCount("review-count-complete", review.completed);
      setReviewCount("review-count-running", review.running);
      setReviewCount("review-count-pending", review.pending);
      setReviewCount("review-count-failed", review.failed);
      const fragment = document.createDocumentFragment();
      for (const reviewer of reviewers) {
        const card = document.createElement("li");
        const status = reviewer.status || "pending";
        card.className = `reviewer-card reviewer-${status.replace(/[^a-z_]/g, "")}`;
        const heading = document.createElement("div");
        heading.className = "reviewer-heading";
        const name = document.createElement("strong");
        name.textContent = reviewer.name || reviewer.id || "Reviewer";
        const badge = document.createElement("span");
        badge.className = `status status-${reviewerStatusClass(status)}`;
        badge.textContent = status;
        heading.append(name, badge);
        card.append(heading);
        for (const value of [reviewer.name && reviewer.name !== reviewer.id ? reviewer.id : "", reviewer.kind, reviewer.path]) {
          if (!value) continue;
          const detail = document.createElement("code");
          detail.textContent = value;
          card.append(detail);
        }
        if (reviewer.summary) {
          const summary = document.createElement("p");
          summary.className = "reviewer-summary";
          summary.textContent = reviewer.summary;
          const openResult = document.createElement("button");
          openResult.type = "button";
          openResult.className = "reviewer-result-open";
          openResult.setAttribute("aria-haspopup", "dialog");
          openResult.textContent = "View full result";
          const fullResult = document.createElement("div");
          fullResult.className = "reviewer-full-result";
          fullResult.hidden = true;
          fullResult.textContent = reviewer.summary;
          card.append(summary, openResult, fullResult);
        }
        fragment.append(card);
      }
      reviewerGrid.replaceChildren(fragment);
      if (reviewerEmpty) {
        reviewerEmpty.hidden = reviewers.length > 0;
        reviewerEmpty.textContent = reviewers.length > 0 ? "" : "Reviewer checkpoints are not available yet.";
      }
    } catch (error) {
      if (reviewerEmpty) {
        reviewerEmpty.hidden = false;
        reviewerEmpty.textContent = error.message;
      }
    } finally {
      reviewRefreshActive = false;
    }
  }

  function follow(operation) {
    if (stream) stream.close();
    currentOperationID = operation.id;
    lastSequence = 0;
    if (cancelButton) cancelButton.hidden = false;
    announce(`Operation ${operation.id} ${operation.state}.`);
    stream = new EventSource(`/api/v1/operations/${encodeURIComponent(operation.id)}/events`);
    stream.onopen = () => announce(`Operation ${operation.id} connected; receiving live progress.`);
    for (const name of ["snapshot", "progress", "warning", "finding", "artifact", "cancel_requested", "recovery_required", "terminal"]) {
      stream.addEventListener(name, (message) => {
        let event;
        try { event = JSON.parse(message.data); } catch { return; }
        appendEvent(name, event);
        if (name === "progress" || name === "snapshot") refreshReviewers();
        if (name === "recovery_required") announce("Some transient progress expired. Refresh durable status for complete truth.");
        if (name === "terminal") {
          if (reviewTimer) clearInterval(reviewTimer);
          stream.close();
          stream = null;
          if (cancelButton) cancelButton.hidden = true;
          announce(`Operation ${event.payload?.state || "finished"}.`);
          window.location.reload();
        }
      });
    }
    stream.onerror = () => {
      if (!stream) return;
      if (stream.readyState === EventSource.CONNECTING) {
        announce("Progress connection was interrupted. Reconnecting automatically…");
        return;
      }
      announce("Live progress is unavailable. Refresh durable status for the authoritative result.", true);
    };
    if (reviewStatus) {
      refreshReviewers();
      if (reviewTimer) clearInterval(reviewTimer);
      reviewTimer = setInterval(refreshReviewers, 2000);
    }
  }

  for (const form of forms) {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        const confirmation = form.closest("[data-stage-panel]")?.querySelector("[data-stage-confirmation]")
          || document.getElementById("operation-confirmation");
        const operation = specification(form, event.submitter);
        announce("Preparing normalized operation scope.");
        const prepared = await command("/api/v1/operations/prepare", {operation});
        if (!confirmation) throw new Error("The run confirmation panel is unavailable.");
        confirmation.hidden = false;
        confirmation.replaceChildren();
        const heading = document.createElement("h3");
        heading.textContent = "Confirm current scope";
        const summary = document.createElement("pre");
        summary.textContent = JSON.stringify({
          operation: prepared.operation,
          affected_paths: prepared.affected_paths,
          mutation_class: prepared.mutation_class,
          prerequisites: prepared.prerequisites,
          expires_at: prepared.expires_at
        }, null, 2);
        const confirmButton = document.createElement("button");
        confirmButton.type = "button";
        confirmButton.textContent = "Confirm and start";
        confirmation.append(heading, summary, confirmButton);
        confirmButton.focus();
        confirmButton.addEventListener("click", async () => {
          confirmButton.disabled = true;
          try {
            const started = await command("/api/v1/operations", {operation, confirmation_token: prepared.confirmation_token});
            const stagePanel = form.closest("[data-stage-panel]");
            if (!stagePanel) {
              window.location.assign(`/operations/${encodeURIComponent(started.id)}`);
              return;
            }
            form.hidden = true;
            confirmation.hidden = true;
            const stageLive = document.createElement("div");
            stageLive.id = "operation-live";
            stageLive.setAttribute("role", "status");
            stageLive.setAttribute("aria-live", "polite");
            const stageTimeline = document.createElement("ol");
            stageTimeline.id = "operation-timeline";
            stageTimeline.className = "operation-timeline";
            stageTimeline.setAttribute("aria-label", "Live stage events");
            const stageCancel = document.createElement("button");
            stageCancel.id = "operation-cancel";
            stageCancel.type = "button";
            stageCancel.textContent = "Cancel run";
            stagePanel.append(stageLive, stageTimeline, stageCancel);
            live = stageLive;
            timeline = stageTimeline;
            cancelButton = stageCancel;
            stageCancel.addEventListener("click", cancelCurrentOperation);
            follow(started);
          } catch (error) {
            confirmButton.disabled = false;
            announce(error.message, true);
          }
        }, {once: true});
      } catch (error) {
        announce(error.message, true);
      }
    });
  }

  async function cancelCurrentOperation() {
    if (!currentOperationID) return;
    cancelButton.disabled = true;
    try {
      const state = await command(`/api/v1/operations/${encodeURIComponent(currentOperationID)}`, null, "DELETE");
      announce(`Cancellation requested; current state is ${state.state}.`);
    } catch (error) {
      announce(error.message, true);
    } finally {
      cancelButton.disabled = false;
    }
  }

  cancelButton?.addEventListener("click", cancelCurrentOperation);

  if (statusRoot) {
    follow({id: statusRoot.dataset.operationId, state: statusRoot.dataset.operationState});
  } else if (reviewStatus) {
    refreshReviewers();
  }

  window.addEventListener("pagehide", () => {
    if (stream) stream.close();
    if (reviewTimer) clearInterval(reviewTimer);
  });
})();
