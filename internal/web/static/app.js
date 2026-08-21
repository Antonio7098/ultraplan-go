(() => {
  "use strict";

  const refreshStatus = document.getElementById("refresh-status");
  for (const link of document.querySelectorAll(".refresh-link")) {
    link.addEventListener("click", () => {
      if (refreshStatus) refreshStatus.textContent = "Refreshing current workspace state.";
    });
  }

  for (const flyout of document.querySelectorAll("[data-nav-flyout]")) {
    const button = flyout.querySelector(":scope > .top-nav-disclosure > button");
    const items = flyout.querySelector("[data-nav-items]");
    let loaded = false;
    let loading = false;

    const loadItems = async () => {
      if (loaded || loading || !items) return;
      loading = true;
      try {
        const response = await fetch(flyout.dataset.endpoint, {headers: {Accept: "application/json"}});
        if (!response.ok) throw new Error(`Request failed with status ${response.status}`);
        const payload = await response.json();
        const entries = Array.isArray(payload.data) ? payload.data : [];
        items.replaceChildren();
        if (!entries.length) {
          const empty = document.createElement("li");
          empty.className = "top-nav-loading";
          empty.textContent = "Nothing here yet.";
          items.append(empty);
        }
        for (const entry of entries) {
          const item = document.createElement("li");
          const link = document.createElement("a");
          link.href = `${flyout.dataset.basePath}/${encodeURIComponent(entry.name)}`;
          link.textContent = entry.name;
          item.append(link);
          items.append(item);
        }
        loaded = true;
      } catch (_) {
        items.firstElementChild.textContent = "Could not load navigation.";
      } finally {
        loading = false;
      }
    };

    const setExpanded = (expanded) => button?.setAttribute("aria-expanded", String(expanded));
    flyout.addEventListener("pointerenter", (event) => {
      if (event.pointerType === "touch") return;
      setExpanded(true);
      loadItems();
    });
    flyout.addEventListener("pointerleave", (event) => {
      if (event.pointerType !== "touch" && !flyout.contains(document.activeElement)) setExpanded(false);
    });
    flyout.addEventListener("focusin", () => {
      setExpanded(true);
      loadItems();
    });
    flyout.addEventListener("focusout", (event) => {
      if (!flyout.contains(event.relatedTarget)) setExpanded(false);
    });
    button?.addEventListener("click", () => {
      setExpanded(true);
      loadItems();
    });
    flyout.addEventListener("keydown", (event) => {
      if (event.key !== "Escape") return;
      setExpanded(false);
      button?.focus();
    });
  }

  for (const control of document.querySelectorAll("[data-add-sprint]")) {
    const open = control.querySelector("[data-add-sprint-open]");
    const dialog = control.querySelector("[data-add-sprint-dialog]");
    const close = control.querySelector("[data-add-sprint-close]");
    open?.addEventListener("click", () => dialog?.showModal());
    close?.addEventListener("click", () => dialog?.close());
    dialog?.addEventListener("click", (event) => {
      if (event.target === dialog) dialog.close();
    });
  }

  for (const mapping of document.querySelectorAll(".smoke-coverage-mapping")) {
    const dialog = mapping.querySelector("[data-coverage-requirement-dialog]");
    const id = dialog?.querySelector("[data-coverage-dialog-id]");
    const status = dialog?.querySelector("[data-coverage-dialog-status]");
    const description = dialog?.querySelector("[data-coverage-dialog-description]");
    const tests = dialog?.querySelector("[data-coverage-dialog-tests]");
    const close = dialog?.querySelector("[data-coverage-dialog-close]");
    let lastTrigger = null;
    const openRequirement = (trigger) => {
      if (!dialog || dialog.open) return;
      lastTrigger = trigger;
      if (id) id.textContent = trigger.dataset.coverageId || "Requirement";
      if (status) {
        status.textContent = trigger.dataset.coverageStatus || "unknown";
        status.className = `status status-${trigger.dataset.coverageStatus === "mapped" ? "ok" : "warn"}`;
      }
      if (description) description.textContent = trigger.dataset.coverageDescription || "No governed description was available.";
      if (tests) tests.textContent = trigger.dataset.coverageTests || "None";
      dialog.showModal();
    };
    for (const trigger of mapping.querySelectorAll(".coverage-requirement-trigger")) {
      trigger.addEventListener("pointerenter", (event) => {
        if (event.pointerType !== "touch") openRequirement(trigger);
      });
      trigger.addEventListener("click", () => openRequirement(trigger));
    }
    close?.addEventListener("click", () => dialog?.close());
    dialog?.addEventListener("click", (event) => {
      if (event.target === dialog) dialog.close();
    });
    dialog?.addEventListener("close", () => lastTrigger?.focus({preventScroll: true}));
  }

  const processes = document.querySelector("[data-running-processes]");
  if (processes) {
    const button = processes.querySelector(":scope > button");
    const count = processes.querySelector("[data-running-count]");
    const status = processes.querySelector("[data-running-status]");
    const items = processes.querySelector("[data-running-items]");
    let loading = false;

    const processLabel = (operation) => {
      const names = {
        "sprint-flow": "Sprint flow", "study-start": "Study loop", "study-resume": "Study loop",
        "sprint-stage": "Sprint stage", "execute-start": "Execution", "execute-resume": "Execution",
        "review-start": "Review", "smoke-start": "Smoke test", "verify-start": "Verification"
      };
      return names[operation.kind] || operation.kind.split("-").map((word) => word[0]?.toUpperCase() + word.slice(1)).join(" ");
    };
    const processScope = (operation) => {
      if (operation.study) return operation.study;
      if (operation.project && operation.sprint) return `${operation.project} / ${operation.sprint}`;
      return operation.project || operation.sprint || "Workspace";
    };
    const durableProcesses = (dashboard) => {
      const active = [];
      const sprints = Array.isArray(dashboard?.sprints) ? dashboard.sprints : dashboard?.slug ? [dashboard] : [];
      const studies = Array.isArray(dashboard?.studies) ? dashboard.studies : dashboard?.name && "run_active" in dashboard ? [dashboard] : [];
      for (const sprint of sprints) {
        const base = `/projects/${encodeURIComponent(sprint.project)}/sprints/${encodeURIComponent(sprint.slug)}/run`;
        if (Number(sprint.execute?.running) > 0) active.push({kind: "execute-start", state: "running", project: sprint.project, sprint: sprint.slug, href: `${base}#stage-execute`});
        if (sprint.review?.status === "running") active.push({kind: "review-start", state: "running", project: sprint.project, sprint: sprint.slug, href: `${base}#stage-review`});
        if (sprint.smoke?.status === "running") active.push({kind: "smoke-start", state: "running", project: sprint.project, sprint: sprint.slug, href: `${base}#stage-smoke`});
        for (const stage of Array.isArray(sprint.stages) ? sprint.stages : []) {
          if (stage.status === "running" && !["execute", "review", "smoke"].includes(stage.name)) active.push({kind: "sprint-stage", state: "running", project: sprint.project, sprint: sprint.slug, href: `${base}#stage-${encodeURIComponent(stage.name)}`});
        }
      }
      for (const study of studies) {
        if (study.run_active) active.push({kind: "study-start", state: "running", study: study.name, href: `/studies/${encodeURIComponent(study.name)}/progress`});
      }
      return active;
    };
    const mergeProcesses = (transient, durable) => {
      const result = [...transient];
      const keys = new Set(transient.map((item) => `${item.kind}:${item.project || ""}:${item.sprint || ""}:${item.study || ""}`));
      const activeFlows = new Set(transient
        .filter((item) => item.kind === "sprint-flow")
        .map((item) => `${item.project || ""}:${item.sprint || ""}`));
      for (const item of durable) {
        const sprintScope = `${item.project || ""}:${item.sprint || ""}`;
        if (activeFlows.has(sprintScope)) continue;
        const key = `${item.kind}:${item.project || ""}:${item.sprint || ""}:${item.study || ""}`;
        if (!keys.has(key)) result.push(item);
      }
      return result;
    };
    const durableStatusPath = () => {
      const sprint = location.pathname.match(/^\/projects\/([^/]+)\/sprints\/([^/]+)/);
      if (sprint) return `/api/v1/projects/${sprint[1]}/sprints/${sprint[2]}`;
      const study = location.pathname.match(/^\/studies\/([^/]+)/);
      return study ? `/api/v1/studies/${study[1]}` : "";
    };
    const render = (operations) => {
      count.textContent = String(operations.length);
      button.setAttribute("aria-label", `Running processes: ${operations.length}`);
      processes.classList.toggle("has-running-processes", operations.length > 0);
      status.textContent = operations.length ? `${operations.length} active` : "None active";
      items.replaceChildren();
      if (!operations.length) {
        const empty = document.createElement("li");
        empty.className = "top-nav-loading";
        empty.textContent = "No processes are running.";
        items.append(empty);
        return;
      }
      for (const operation of operations) {
        const item = document.createElement("li");
        const link = document.createElement("a");
        const title = document.createElement("strong");
        const detail = document.createElement("span");
        link.href = operation.href || `/operations/${encodeURIComponent(operation.id)}`;
        title.textContent = processLabel(operation);
        detail.textContent = `${processScope(operation)} · ${operation.state}`;
        link.append(title, detail);
        item.append(link);
        items.append(item);
      }
    };
    const load = async () => {
      if (loading || document.hidden) return;
      loading = true;
      try {
        const statusPath = durableStatusPath();
        const responses = await Promise.all([
          fetch("/api/v1/operations", {headers: {Accept: "application/json"}}),
          statusPath ? fetch(statusPath, {headers: {Accept: "application/json"}}) : Promise.resolve(null)
        ]);
        const [operationsResponse, durableResponse] = responses;
        if (!operationsResponse.ok || (durableResponse && !durableResponse.ok)) throw new Error();
        const [operationsPayload, durablePayload] = await Promise.all([operationsResponse.json(), durableResponse ? durableResponse.json() : Promise.resolve({data: {}})]);
        const transient = Array.isArray(operationsPayload.data) ? operationsPayload.data : [];
        render(mergeProcesses(transient, durableProcesses(durablePayload.data)));
      } catch (_) {
        status.textContent = "Unavailable";
      } finally {
        loading = false;
      }
    };
    const setExpanded = (expanded) => button.setAttribute("aria-expanded", String(expanded));
    processes.addEventListener("pointerenter", (event) => {
      if (event.pointerType === "touch") return;
      setExpanded(true);
      load();
    });
    processes.addEventListener("pointerleave", (event) => {
      if (event.pointerType !== "touch" && !processes.contains(document.activeElement)) setExpanded(false);
    });
    processes.addEventListener("focusin", () => {
      setExpanded(true);
      load();
    });
    processes.addEventListener("focusout", (event) => {
      if (!processes.contains(event.relatedTarget)) setExpanded(false);
    });
    button.addEventListener("click", () => {
      const expanded = button.getAttribute("aria-expanded") !== "true";
      setExpanded(expanded);
      if (expanded) load();
    });
    processes.addEventListener("keydown", (event) => {
      if (event.key === "Escape") { setExpanded(false); button.focus(); }
    });
    document.addEventListener("click", (event) => {
      if (!processes.contains(event.target)) setExpanded(false);
    });
    document.addEventListener("visibilitychange", () => { if (!document.hidden) load(); });
    load();
    window.setInterval(load, 10000);
  }

  for (const stack of document.querySelectorAll("[data-sidebar-stack]")) {
    const launcher = stack.querySelector("[data-sidebar-toggle]");
    const pin = stack.querySelector("[data-sidebar-pin]");
    const label = stack.querySelector("[data-sidebar-label]");
    const pinLabel = stack.querySelector("[data-pin-label]");
    const storageKey = "ultraplan.sidebar.pinned";
    let pinned = false;
    let hovered = false;
    try { pinned = localStorage.getItem(storageKey) === "true"; } catch (_) {}
    stack.classList.add("is-collapsible");
    stack.classList.toggle("is-pinned", pinned);
    stack.classList.toggle("is-expanded", pinned);
    pin?.setAttribute("aria-pressed", String(pinned));
    if (pinLabel) pinLabel.textContent = pinned ? "Unpin navigation" : "Pin navigation";
    launcher?.setAttribute("aria-expanded", String(pinned));

    const setExpanded = (expanded) => {
      if (pinned && !expanded) return;
      stack.classList.toggle("is-expanded", expanded);
      launcher?.setAttribute("aria-expanded", String(expanded));
    };
    stack.addEventListener("pointerenter", (event) => {
      if (event.pointerType === "touch") return;
      hovered = true;
      setExpanded(true);
    });
    stack.addEventListener("pointerleave", (event) => {
      if (event.pointerType === "touch") return;
      hovered = false;
      setExpanded(false);
    });
    stack.addEventListener("focusin", () => setExpanded(true));
    stack.addEventListener("focusout", (event) => {
      if (!stack.contains(event.relatedTarget)) setExpanded(false);
    });
    launcher?.addEventListener("click", () => setExpanded(true));
    pin?.addEventListener("click", () => {
      pinned = !pinned;
      stack.classList.toggle("is-pinned", pinned);
      stack.classList.toggle("is-expanded", pinned || hovered || stack.matches(":focus-within"));
      pin.setAttribute("aria-pressed", String(pinned));
      launcher?.setAttribute("aria-expanded", String(stack.classList.contains("is-expanded")));
      if (pinLabel) pinLabel.textContent = pinned ? "Unpin navigation" : "Pin navigation";
      try { localStorage.setItem(storageKey, String(pinned)); } catch (_) {}
    });

    const showPanel = (id) => {
      const target = stack.querySelector(`#${CSS.escape(id)}`);
      if (!target) return false;
      for (const panel of stack.querySelectorAll("[data-sidebar-panel]")) panel.hidden = panel !== target;
      const heading = target.querySelector("h2")?.textContent?.trim();
      if (label && heading) label.textContent = heading;
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
      // Drill-down links must retain normal navigation so the destination's
      // main content changes as well as its contextual sidebar. Only the back
      // buttons above are sidebar-local controls.
    });
  }

  for (const details of document.querySelectorAll(".detail-sidebar details")) {
    let pinnedOpen = details.open;
    details.addEventListener("pointerenter", (event) => {
      if (event.pointerType === "touch") return;
      if (!pinnedOpen) details.classList.add("sidebar-hover-preview");
    });
    details.addEventListener("pointerleave", (event) => {
      if (event.pointerType === "touch") return;
      details.classList.remove("sidebar-hover-preview");
    });
    details.querySelector(":scope > summary")?.addEventListener("click", (event) => {
      event.preventDefault();
      pinnedOpen = !pinnedOpen;
      details.classList.remove("sidebar-hover-preview");
      details.open = pinnedOpen;
    });
  }

  for (const workspace of document.querySelectorAll("[data-stage-workspace]")) {
    const controls = [...workspace.querySelectorAll("[data-stage-select]")];
    const panels = [...workspace.querySelectorAll("[data-stage-panel]")];
    const artifactBrowser = workspace.querySelector("[data-previous-artifacts]");
    const artifactLinks = [...(artifactBrowser?.querySelectorAll("[data-artifact-select]") || [])];
    const artifactEmpty = artifactBrowser?.querySelector("[data-artifact-empty]");
    const artifactContent = artifactBrowser?.querySelector("[data-artifact-content]");
    const artifactName = artifactBrowser?.querySelector("[data-artifact-name]");
    const artifactMeta = artifactBrowser?.querySelector("[data-artifact-meta]");
    const artifactSource = artifactBrowser?.querySelector("[data-artifact-source]");
    const artifactOpen = artifactBrowser?.querySelector("[data-artifact-open]");
    const artifactCache = new Map();
    const unavailableArtifacts = new Set();
    let artifactRequest = 0;

    const showArtifact = async (link, fallbacks = []) => {
      const request = ++artifactRequest;
      for (const item of artifactLinks) item.setAttribute("aria-current", String(item === link));
      if (artifactEmpty) {
        artifactEmpty.hidden = false;
        artifactEmpty.textContent = `Loading ${link.dataset.artifactStage}…`;
      }
      if (artifactContent) artifactContent.hidden = true;
      try {
        let artifact = artifactCache.get(link.dataset.artifactRef);
        if (!artifact) {
          const response = await fetch(`/api/v1/artifacts/${encodeURIComponent(link.dataset.artifactRef)}`, {headers: {Accept: "application/json"}});
          if (!response.ok) {
            const error = new Error(`Request failed with status ${response.status}`);
            error.status = response.status;
            throw error;
          }
          artifact = (await response.json()).data;
          artifactCache.set(link.dataset.artifactRef, artifact);
        }
        if (request !== artifactRequest) return;
        if (artifactName) artifactName.textContent = artifact.display_path || link.dataset.artifactStage;
        if (artifactMeta) artifactMeta.textContent = `${artifact.media_type} · ${artifact.returned_bytes} of ${artifact.size_bytes} bytes${artifact.truncated ? " · truncated" : ""}`;
        if (artifactSource) artifactSource.textContent = artifact.content || "";
        if (artifactOpen) artifactOpen.href = link.href;
        if (artifactEmpty) artifactEmpty.hidden = true;
        if (artifactContent) artifactContent.hidden = false;
      } catch (error) {
        if (request !== artifactRequest) return;
        if (error.status === 404) {
          unavailableArtifacts.add(link.dataset.artifactRef);
          link.closest("[data-artifact-item]").hidden = true;
          const fallback = fallbacks[fallbacks.length - 1];
          if (fallback) {
            showArtifact(fallback, fallbacks.slice(0, -1));
            return;
          }
        }
        if (artifactEmpty) {
          artifactEmpty.hidden = false;
          artifactEmpty.textContent = "No previous artefact preview is available for this stage.";
        }
      }
    };

    for (const link of artifactLinks) link.addEventListener("click", (event) => {
      event.preventDefault();
      showArtifact(link);
    });

    const updateArtifacts = (stageID) => {
      const currentIndex = controls.findIndex((control) => control.dataset.stageSelect === stageID);
      const available = [];
      for (const link of artifactLinks) {
        const artifactIndex = controls.findIndex((control) => control.dataset.stageSelect === `stage-${link.dataset.artifactStage}`);
        const artifactWasProduced = controls[artifactIndex]?.dataset.stageHasArtifact === "true";
        const visible = artifactIndex >= 0 && artifactIndex < currentIndex && artifactWasProduced && !unavailableArtifacts.has(link.dataset.artifactRef);
        link.closest("[data-artifact-item]").hidden = !visible;
        if (visible) available.push(link);
      }
      const selected = available.find((link) => link.getAttribute("aria-current") === "true");
      if (selected) return;
      if (available.length) {
        showArtifact(available[available.length - 1], available.slice(0, -1));
        return;
      }
      artifactRequest++;
      for (const link of artifactLinks) link.setAttribute("aria-current", "false");
      if (artifactContent) artifactContent.hidden = true;
      if (artifactEmpty) {
        artifactEmpty.hidden = false;
        artifactEmpty.textContent = "No artefacts from previous stages yet.";
      }
    };

    const selectStage = (id, moveFocus = false) => {
      const panel = panels.find((item) => item.id === id);
      if (!panel) return;
      for (const item of panels) item.hidden = item !== panel;
      for (const control of controls) {
        const selected = control.dataset.stageSelect === id;
        control.setAttribute("aria-selected", String(selected));
        control.tabIndex = selected ? 0 : -1;
      }
      updateArtifacts(id);
      if (moveFocus) panel.focus();
      history.replaceState(null, "", `#${id}`);
    };
    for (const control of controls) control.addEventListener("click", (event) => {
      event.preventDefault();
      selectStage(control.dataset.stageSelect, true);
    });
    const requested = location.hash.slice(1);
    const initial = panels.some((item) => item.id === requested)
      ? requested
      : controls.find((control) => control.closest(".stage-running"))?.dataset.stageSelect
        || controls.find((control) => !control.closest(".stage-complete, .stage-completed, .stage-skipped"))?.dataset.stageSelect
        || controls[controls.length - 1]?.dataset.stageSelect;
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
  const activityTime = document.getElementById("activity-time");
  const activityAgents = document.getElementById("activity-agents");
  const activityActions = document.getElementById("activity-actions");
  const activityTools = document.getElementById("activity-tools");
  const latestEvent = document.getElementById("latest-event");
  const eventLogCount = document.getElementById("event-log-count");
  let stream = null;
  let reviewTimer = null;
  let reviewRefreshActive = false;
  const reviewerStates = new Map();
  let reviewCounts = "";
  let activityStartedAt = null;
  let actionCount = Number(activityActions?.textContent || 0);
  let toolCount = 0;
  const activeAgents = new Set();
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
    if (window.UltraPlanOperations) return window.UltraPlanOperations.command(path, payload, method);
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
    const message = friendlyEvent(payload, name);
    item.textContent = `${context ? `[${context}] ` : ""}${message}${progress}`;
    item.dataset.event = name;
    timeline.append(item);
    while (timeline.children.length > 100) timeline.firstElementChild.remove();
    timeline.scrollTop = timeline.scrollHeight;
    recordActivity(message, payload, event.time);
  }

  function friendlyEvent(payload, fallback) {
    if (payload.event_kind === "tool") return `Used ${payload.tool || "a tool"}${payload.action ? ` · ${payload.action}` : ""}`;
    if (payload.event_kind === "artifact") return "Produced an artifact";
    if (payload.event_kind === "usage") return "Updated usage totals";
    if (payload.event_kind === "permission") return "Checked tool permissions";
    if (payload.event_kind === "retry") return "Retrying the agent run";
    if (payload.event_kind === "lifecycle") return payload.action ? `Agent is ${String(payload.action).replaceAll("_", " ")}` : "Agent status changed";
    return payload.message || payload.state || payload.reason || fallback;
  }

  function recordActivity(message, payload = {}, time = "") {
    if (latestEvent) latestEvent.textContent = message;
    if (payload.task) activeAgents.add(payload.task);
    if (activeAgents.size && activityAgents) activityAgents.textContent = String(activeAgents.size);
    actionCount++;
    if (activityActions) activityActions.textContent = String(actionCount);
    if (payload.event_kind === "tool") {
      toolCount++;
      if (activityTools) activityTools.textContent = String(toolCount);
    }
    if (!activityStartedAt && time) activityStartedAt = new Date(time);
    if (eventLogCount && timeline) eventLogCount.textContent = String(timeline.children.length);
  }

  function updateActivityTime() {
    if (!activityTime || !activityStartedAt || Number.isNaN(activityStartedAt.getTime())) return;
    const seconds = Math.max(0, Math.floor((Date.now() - activityStartedAt.getTime()) / 1000));
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    activityTime.textContent = hours ? `${hours}h ${minutes}m` : minutes ? `${minutes}m` : `${seconds}s`;
  }
  window.setInterval(updateActivityTime, 1000);

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

  function appendReviewProgress(message) {
    if (!timeline) return;
    const item = document.createElement("li");
    item.textContent = `[review] ${message}`;
    item.dataset.event = "durable-review";
    timeline.append(item);
    while (timeline.children.length > 100) timeline.firstElementChild.remove();
    timeline.scrollTop = timeline.scrollHeight;
    if (latestEvent) latestEvent.textContent = message;
    if (eventLogCount) eventLogCount.textContent = String(timeline.children.length);
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
      if (review.started_at) {
        activityStartedAt = new Date(review.started_at);
        updateActivityTime();
      }
      if (activityAgents) activityAgents.textContent = String(review.total || reviewers.length || 0);
      if (activityActions) activityActions.textContent = String(review.completed || 0);
      setReviewCount("review-count-complete", review.completed);
      setReviewCount("review-count-running", review.running);
      setReviewCount("review-count-pending", review.pending);
      setReviewCount("review-count-failed", review.failed);
      const counts = `${review.completed || 0}/${review.total || reviewers.length} complete · ${review.running || 0} running · ${review.pending || 0} pending · ${review.failed || 0} failed`;
      if (counts !== reviewCounts) {
        reviewCounts = counts;
        appendReviewProgress(counts);
      }
      const fragment = document.createDocumentFragment();
      for (const reviewer of reviewers) {
        const card = document.createElement("li");
        const status = reviewer.status || "pending";
        const previousStatus = reviewerStates.get(reviewer.id);
        if ((previousStatus && previousStatus !== status) || (!previousStatus && status === "running")) {
          appendReviewProgress(`${reviewer.name || reviewer.id || "Reviewer"} ${status}`);
        }
        reviewerStates.set(reviewer.id, status);
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
    window.UltraPlanSSE?.closeOnAbort(stream);
    stream.onopen = () => announce(`Operation ${operation.id} connected; receiving live progress.`);
    for (const name of window.UltraPlanSSE?.stableEvents || ["snapshot", "progress", "warning", "finding", "artifact", "cancel_requested", "recovery_required", "terminal"]) {
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
        const stagePanel = form.closest("[data-stage-panel]");
        const stageStatus = stagePanel?.querySelector("[data-stage-operation-status]");
        if (stageStatus) live = stageStatus;
        const confirmation = stagePanel?.querySelector("[data-stage-confirmation]")
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
    reviewTimer = setInterval(refreshReviewers, 2000);
  }

  window.addEventListener("pagehide", () => {
    if (stream) stream.close();
    if (reviewTimer) clearInterval(reviewTimer);
  });
})();
