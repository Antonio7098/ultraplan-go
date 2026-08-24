(() => {
  "use strict";

  const refreshStatus = document.getElementById("refresh-status");
  for (const link of document.querySelectorAll(".refresh-link")) {
    link.addEventListener("click", () => {
      if (refreshStatus) refreshStatus.textContent = "Refreshing current workspace state.";
    });
  }

  for (const form of document.querySelectorAll("form[data-confirm]")) {
    form.addEventListener("submit", (event) => {
      if (window.confirm(form.dataset.confirm || "Continue?")) return;
      event.preventDefault();
      event.submitter?.focus();
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
    let closeTimer = null;
    const cancelClose = () => {
      if (closeTimer) window.clearTimeout(closeTimer);
      closeTimer = null;
    };
    const closeRequirement = () => {
      cancelClose();
      if (lastTrigger) lastTrigger.setAttribute("aria-expanded", "false");
      if (dialog) dialog.hidden = true;
      lastTrigger = null;
    };
    const scheduleClose = () => {
      cancelClose();
      closeTimer = window.setTimeout(closeRequirement, 140);
    };
    const openRequirement = (trigger) => {
      if (!dialog) return;
      cancelClose();
      if (lastTrigger && lastTrigger !== trigger) lastTrigger.setAttribute("aria-expanded", "false");
      lastTrigger = trigger;
      trigger.setAttribute("aria-expanded", "true");
      if (id) id.textContent = trigger.dataset.coverageId || "Requirement";
      if (status) {
        status.textContent = trigger.dataset.coverageStatus || "unknown";
        status.className = `status status-${trigger.dataset.coverageStatus === "mapped" ? "ok" : "warn"}`;
      }
      if (description) description.textContent = trigger.dataset.coverageDescription || "No governed description was available.";
      if (tests) tests.textContent = trigger.dataset.coverageTests || "None";
      dialog.hidden = false;
      const anchor = trigger.getBoundingClientRect();
      const panel = dialog.getBoundingClientRect();
      const gap = 8;
      let left = anchor.right + gap;
      if (left + panel.width > window.innerWidth - gap) left = anchor.left - panel.width - gap;
      left = Math.max(gap, Math.min(left, window.innerWidth - panel.width - gap));
      let top = anchor.top;
      if (top + panel.height > window.innerHeight - gap) top = window.innerHeight - panel.height - gap;
      dialog.style.left = `${Math.max(gap, left)}px`;
      dialog.style.top = `${Math.max(gap, top)}px`;
    };
    for (const trigger of mapping.querySelectorAll(".coverage-requirement-trigger")) {
      trigger.setAttribute("aria-expanded", "false");
      trigger.addEventListener("pointerenter", (event) => {
        if (event.pointerType !== "touch") openRequirement(trigger);
      });
      trigger.addEventListener("pointerleave", scheduleClose);
      trigger.addEventListener("focus", () => openRequirement(trigger));
      trigger.addEventListener("blur", scheduleClose);
      trigger.addEventListener("click", () => openRequirement(trigger));
    }
    dialog?.addEventListener("pointerenter", cancelClose);
    dialog?.addEventListener("pointerleave", scheduleClose);
    dialog?.addEventListener("focusin", cancelClose);
    dialog?.addEventListener("focusout", scheduleClose);
    close?.addEventListener("click", closeRequirement);
    mapping.addEventListener("keydown", (event) => {
      if (event.key === "Escape") closeRequirement();
    });
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
      return names[operation.kind] || (operation.kind || "run").split("-").map((word) => word[0]?.toUpperCase() + word.slice(1)).join(" ");
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
    let renderedKey = "";
    const render = (operations) => {
      const key = JSON.stringify(operations);
      if (key === renderedKey) return;
      renderedKey = key;
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
        if (operation.kind === "study-loop") {
          title.textContent = `Study · ${operation.study}`;
          detail.textContent = `${operation.agents} parallel agent${operation.agents === 1 ? "" : "s"}`;
        } else {
          title.textContent = processLabel(operation);
          detail.textContent = `${processScope(operation)} · ${operation.state}`;
        }
        link.append(title, detail);
        item.append(link);
        items.append(item);
      }
    };
    const groupActiveRuns = (runs) => {
      const grouped = [];
      const studies = new Map();
      for (const run of runs) {
        const target = run.target || {};
        if (!target.study || target.project || target.sprint) {
          grouped.push(run);
          continue;
        }
        const existing = studies.get(target.study);
        if (existing) {
          if (target.kind === "operation" && !existing.loopRunID) existing.loopRunID = run.run_id;
          else existing.agents++;
          continue;
        }
        const entry = {kind: "study-loop", study: target.study, state: run.lifecycle, loopRunID: target.kind === "operation" ? run.run_id : "", agents: target.kind === "operation" ? 0 : 1};
        studies.set(target.study, entry);
        grouped.push(entry);
      }
      return grouped;
    };
    const load = async () => {
      if (loading || document.hidden) return;
      loading = true;
      try {
        const response = await fetch("/api/v1/runs?lifecycle=accepted,queued,running,cancelling&limit=50", {headers: {Accept: "application/json"}});
        if (!response.ok) throw new Error();
        const payload = await response.json();
        const runs = Array.isArray(payload?.data?.runs) ? payload.data.runs : [];
        render(groupActiveRuns(runs).map((run) => run.kind === "study-loop" ? {
          id: run.loopRunID,
          kind: run.kind,
          study: run.study,
          agents: run.agents,
          href: run.loopRunID
            ? `/runs/${encodeURIComponent(run.loopRunID)}`
            : `/studies/${encodeURIComponent(run.study)}/progress`
        } : {
          id: run.run_id,
          kind: run.target?.operation || run.target?.kind || "run",
          state: run.lifecycle,
          project: run.target?.project,
          sprint: run.target?.sprint,
          study: run.target?.study,
          href: `/runs/${encodeURIComponent(run.run_id)}`
        }));
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
    window.setInterval(load, 5000);
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

    const byteLabel = (value) => `${Number(value || 0).toLocaleString()} bytes`;
    for (const disclosure of workspace.querySelectorAll("[data-prompt-observability]")) {
      let loaded = false;
      let loading = false;
      disclosure.addEventListener("toggle", async () => {
        if (!disclosure.open || loaded || loading) return;
        loading = true;
        const pending = disclosure.querySelector("[data-prompt-loading]");
        const result = disclosure.querySelector("[data-prompt-result]");
        const unavailable = disclosure.querySelector("[data-prompt-unavailable]");
        if (pending) pending.textContent = "Preparing content-free prompt summary…";
        try {
          const response = await fetch(disclosure.dataset.endpoint, {headers: {Accept: "application/json"}});
          if (!response.ok) throw new Error(`Request failed with status ${response.status}`);
          const bundle = (await response.json()).data;
          const setText = (selector, value) => {
            const element = disclosure.querySelector(selector);
            if (element) element.textContent = value;
          };
          setText("[data-prompt-scope]", bundle.scope || "Deterministic stage preview");
          if (!bundle.available) {
            if (unavailable) {
              unavailable.hidden = false;
              unavailable.textContent = bundle.unavailable_reason || "This bundle cannot be prepared until its required inputs are available.";
            }
            if (result) result.hidden = true;
            if (pending) pending.hidden = true;
            loaded = true;
            return;
          }
          setText("[data-prompt-total]", byteLabel(bundle.total_bytes));
          setText("[data-prompt-prefix]", byteLabel(bundle.shared_prefix_bytes));
          setText("[data-prompt-suffix]", byteLabel(bundle.stage_suffix_bytes));
          setText("[data-prompt-cache]", bundle.cache_candidate ? "yes" : "no");
          setText("[data-prompt-transport]", bundle.cache_transport || "none");
          setText("[data-prompt-key]", bundle.cache_key || "none");
          setText("[data-prompt-digest]", bundle.shared_prefix_sha256 || "none");
          const blocks = disclosure.querySelector("[data-prompt-blocks]");
          blocks?.replaceChildren();
          for (const block of Array.isArray(bundle.blocks) ? bundle.blocks : []) {
            const item = document.createElement("li");
            const name = document.createElement("strong");
            const metadata = document.createElement("span");
            name.textContent = block.id;
            metadata.textContent = `${block.kind}${block.mode ? `/${block.mode}` : ""} · ${byteLabel(block.bytes)} · ${block.cacheable ? "stable/cacheable" : "stage-specific"} · ${block.sha256}`;
            item.append(name, metadata);
            blocks?.append(item);
          }
          if (pending) pending.hidden = true;
          if (unavailable) unavailable.hidden = true;
          if (result) result.hidden = false;
          loaded = true;
        } catch (_) {
          if (pending) pending.hidden = true;
          if (unavailable) {
            unavailable.hidden = false;
            unavailable.textContent = "The prompt summary could not be loaded. The stage input contract above is still authoritative.";
          }
        } finally {
          loading = false;
        }
      });
    }

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
  if (forms.length === 0 && !statusRoot && !reviewStatus && !document.querySelector(".reviewer-card") && !document.querySelector("[data-run-id]")) return;

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
    const selectedModel = form.elements?.model?.value;
    if (selectedModel) options.model = selectedModel;
    const selectedParallelism = form.elements?.parallelism?.value || form.dataset.parallelism;
    if (selectedParallelism) options.parallelism = Number(selectedParallelism);
    return {kind: submitter?.dataset.operationKind || form.dataset.operationKind, scope, options};
  }

  async function loadModelChoices() {
    const inputs = [...document.querySelectorAll("input[data-model-input]")];
    if (inputs.length === 0 || !window.UltraPlanOperations) return;
    let data;
    try {
      data = await command("/api/v1/models", null, "GET");
    } catch {
      return;
    }
    const list = document.getElementById("ultraplan-model-options");
    if (list) {
      const models = Array.isArray(data?.models) ? data.models : [];
      for (const model of models) {
        const reference = [model.provider, model.id].filter(Boolean).join("/");
        if (!reference || list.querySelector(`option[value="${CSS.escape(reference)}"]`)) continue;
        const option = document.createElement("option");
        option.value = reference;
        if (reference === data.default) option.label = `${reference} (workspace default)`;
        list.appendChild(option);
      }
    }
    if (data?.default) {
      for (const input of inputs) {
        input.placeholder = `Workspace default (${data.default})`;
      }
    }
  }

  void loadModelChoices();

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

  const durableRun = document.querySelector("[data-run-id]");
  const durableTimeline = document.querySelector("[data-run-timeline]");
  const durableLive = document.querySelector("[data-run-live-status]");
  let durableStream = null;
  let durableReconnect = null;
  let durableTerminal = false;
  let durableLast = Number(durableRun?.dataset.lastSequence || 0);
  let durableLiveTimer = null;
  let durableLivePending = "";
  let durableLiveUpdatedAt = 0;

  const setDurableLive = (message) => {
    if (!durableLive) return;
    durableLivePending = message;
    const remaining = Math.max(0, 250 - (Date.now() - durableLiveUpdatedAt));
    if (durableLiveTimer) return;
    const update = () => {
      durableLiveTimer = null;
      durableLive.textContent = durableLivePending;
      durableLiveUpdatedAt = Date.now();
    };
    if (remaining === 0) update();
    else durableLiveTimer = window.setTimeout(update, remaining);
  };
  const agentSection = document.querySelector("[data-run-agents]");
  const agentGrid = document.getElementById("run-agent-grid");
  const agentEmptyMessage = document.getElementById("run-agent-grid-empty");
  const completedToggle = document.querySelector("[data-run-completed-toggle]");
  const completedToggleLabel = completedToggle?.querySelector("[data-run-completed-label]");
  const completedToggleCount = completedToggle?.querySelector("[data-run-completed-count]");
  const completedTogglePlural = completedToggle?.querySelector("[data-run-completed-plural]");
  const agentDialog = document.getElementById("run-agent-dialog");
  const agentDialogClose = document.getElementById("run-agent-dialog-close");
  const agentDialogTitle = document.getElementById("run-agent-dialog-title");
  const agentDialogSubtitle = document.getElementById("run-agent-dialog-subtitle");
  const agentDialogSummary = document.getElementById("run-agent-dialog-summary");
  const agentDialogTimeline = document.getElementById("run-agent-dialog-timeline");
  const runAgents = new Map();
  let openAgentTask = "";
  const seenAgentTasks = new Set();
  let runAgentsLive = false;
  let completedAgentsExpanded = false;
  let previousActiveAgentCount = 0;
  const agentFailures = new Map();
  let agentFailuresFetch;
  for (const payload of document.querySelectorAll("script[data-run-agent-failures]")) {
    for (const failure of JSON.parse(payload.textContent || "[]")) {
      if (failure?.task && failure.message) agentFailures.set(failure.task, {code: failure.code || "", message: failure.message});
    }
  }
  const seededAgentStatus = (status) => ({completed: "completed", failed: "failed", cancelled: "failed", retrying: "pending", waiting: "pending", running: "running", validating: "running"}[status]);
  const agentFacts = new Map();
  for (const payload of document.querySelectorAll("script[data-run-agent-tasks]")) {
    for (const item of JSON.parse(payload.textContent || "[]")) {
      if (!item?.task) continue;
      agentFacts.set(item.task, item);
      const status = seededAgentStatus(item.status);
      if (!status || runAgents.has(item.task)) continue;
      runAgents.set(item.task, {task: item.task, events: [], toolCalls: 0, status, latest: null});
    }
  }
  const formatRunWait = (ms) => {
    let seconds = Math.max(0, Math.round(ms / 1000));
    const parts = [];
    for (const [size, unit] of [[3600, "h"], [60, "m"], [1, "s"]]) {
      const value = Math.floor(seconds / size);
      seconds -= value * size;
      if (value || parts.length) parts.push(`${value}${unit}`);
    }
    return parts.join(" ") || "0s";
  };
  const agentRetryWait = (agent) => {
    const facts = agentFacts.get(agent.task);
    const retryAt = facts?.retry_after ? new Date(facts.retry_after).getTime() : NaN;
    if (!Number.isFinite(retryAt) || retryAt <= Date.now()) return "";
    return `Next retry in ${formatRunWait(retryAt - Date.now())} · ${facts.retries || Math.max(0, (facts.attempts || 1) - 1)} retr${(facts.retries || Math.max(0, (facts.attempts || 1) - 1)) === 1 ? "y" : "ies"} so far`;
  };
  const loadAgentFailures = () => {
    if (agentFailuresFetch) return agentFailuresFetch;
    const source = document.querySelector("script[data-run-agent-failures]");
    const study = source?.dataset.study;
    agentFailuresFetch = study ? fetch(`/api/v1/studies/${encodeURIComponent(study)}`, {headers: {Accept: "application/json"}})
      .then((response) => response.ok ? response.json() : null)
      .then((body) => {
        for (const failure of body?.data?.failures || []) {
          if (failure?.task && failure.message) agentFailures.set(failure.task, {code: failure.code || "", message: failure.message});
        }
        renderAgentGrid();
        if (openAgentTask) refreshAgentDialogSummary(runAgents.get(openAgentTask));
      })
      .catch(() => {}) : Promise.resolve();
    return agentFailuresFetch;
  };

  const humanizeRunText = (value) => String(value || "").replaceAll("_", " ");
  const formatRunTime = (value) => {
    if (!value) return "";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "" : date.toLocaleTimeString(undefined, {hour12: false});
  };
  const runAgentIdentity = (task) => {
    const parts = String(task || "").split(":");
    const dimension = parts.length >= 4 && parts[2] && parts[3] ? `${parts[2]}-${parts[3]}` : "";
    const source = parts.length >= 6 ? parts[4] : "";
    const title = dimension ? (source ? `${dimension} · ${source}` : dimension) : task;
    return {kind: parts[0] || "agent", dimension, source, title};
  };
  const describeRunEvent = (event) => {
    const payload = event.payload || {};
    if (payload.kind === "tool") return `Tool call${payload.tool ? ` · ${payload.tool}` : ""}`;
    const labels = {started: "Agent started", completed: "Report completed", failed: "Agent failed", waiting: "Waiting to retry", cancelled: "Cancelled", validating: "Validating report"};
    if (labels[event.stage]) return labels[event.stage];
    if (event.stage === "runtime" && payload.action) return humanizeRunText(payload.action);
    return event.stage ? humanizeRunText(event.stage) : humanizeRunText(event.type || "event");
  };
  const runAgentStatusFor = (event) => {
    switch (event.stage) {
      case "started": case "runtime": return "running";
      case "validating": case "waiting": return "pending";
      case "completed": return "completed";
      case "failed": case "cancelled": return "failed";
      default: return "";
    }
  };
  const runAgentBadge = (status) => status === "completed" ? "ok" : status === "running" ? "info" : status === "pending" ? "warn" : "error";

  const appendAgentStreamRow = (list, entry) => {
    const item = document.createElement("li");
    const time = formatRunTime(entry.time);
    item.textContent = `${time ? `${time} · ` : ""}${entry.label}${entry.detail ? ` (${entry.detail})` : ""}`;
    list.append(item);
    while (list.children.length > 500) list.firstElementChild.remove();
    list.scrollTop = list.scrollHeight;
  };

  const runAgentIsActive = (agent) => agent.status === "running" || agent.status === "pending";

  const activeAgentCount = () => {
    let count = 0;
    for (const agent of runAgents.values()) if (runAgentIsActive(agent)) count++;
    return count;
  };

  const flipAgentGrid = (mutate) => {
    if (!agentGrid || !runAgentsLive || typeof agentGrid.animate !== "function") {
      mutate();
      return;
    }
    const before = new Map();
    for (const card of agentGrid.children) before.set(card.dataset.runAgent, card.getBoundingClientRect());
    mutate();
    for (const card of agentGrid.children) {
      const previous = before.get(card.dataset.runAgent);
      if (!previous) continue;
      const now = card.getBoundingClientRect();
      const dx = previous.left - now.left;
      const dy = previous.top - now.top;
      if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;
      card.animate(
        [{transform: `translate(${dx}px, ${dy}px)`}, {transform: "translate(0, 0)"}],
        {duration: 340, easing: "cubic-bezier(.16, 1, .3, 1)"}
      );
    }
  };

  const renderAgentGrid = () => {
    if (!agentGrid || !agentSection) return;
    if (previousActiveAgentCount > 0 && activeAgentCount() === 0) completedAgentsExpanded = true;
    const active = [];
    const settled = [];
    let activeCount = 0;
    for (const agent of runAgents.values()) {
      if (runAgentIsActive(agent)) {
        active.push(agent);
        activeCount++;
      } else settled.push(agent);
    }
    previousActiveAgentCount = activeCount;
    const showCompleted = completedAgentsExpanded || activeCount === 0;
    const fragment = document.createDocumentFragment();
    for (const agent of [...active, ...(showCompleted ? settled : [])]) {
      const identity = runAgentIdentity(agent.task);
      const card = document.createElement("li");
      card.className = `reviewer-card reviewer-${agent.status} run-agent-card`;
      card.dataset.runAgent = agent.task;
      if (!seenAgentTasks.has(agent.task)) {
        seenAgentTasks.add(agent.task);
        if (runAgentsLive) card.classList.add("agent-card-new");
      }
      if (runAgentIsActive(agent)) card.classList.add("agent-card-live");
      const heading = document.createElement("div");
      heading.className = "reviewer-heading";
      const name = document.createElement("strong");
      name.textContent = identity.title || agent.task;
      const badge = document.createElement("span");
      badge.className = `status status-${runAgentBadge(agent.status)}`;
      badge.textContent = agent.status;
      heading.append(name, badge);
      card.append(heading);
      const taskCode = document.createElement("code");
      taskCode.textContent = agent.task;
      card.append(taskCode);
      const latest = document.createElement("p");
      latest.className = "reviewer-summary agent-latest";
      latest.textContent = agent.latest?.label || "Awaiting committed activity.";
      card.append(latest);
      const failure = agentFailures.get(agent.task);
      if (agent.status === "failed" && failure) {
        const reason = document.createElement("p");
        reason.className = "agent-failure";
        reason.textContent = failure.code ? `${failure.code}: ${failure.message}` : failure.message;
        card.append(reason);
      }
      const retryWait = agentRetryWait(agent);
      if (retryWait) {
        const wait = document.createElement("p");
        wait.className = "agent-retry-wait";
        wait.textContent = retryWait;
        card.append(wait);
      }
      const meta = document.createElement("div");
      meta.className = "agent-meta";
      const time = document.createElement("span");
      time.textContent = formatRunTime(agent.latest?.time) || "no time yet";
      const tools = document.createElement("span");
      tools.textContent = `${agent.toolCalls} tool call${agent.toolCalls === 1 ? "" : "s"}`;
      const total = document.createElement("span");
      total.textContent = `${agent.events.length} events`;
      meta.append(time, tools, total);
      const facts = agentFacts.get(agent.task);
      for (const [label, value] of facts ? [["Provider", facts.provider], ["Model", facts.model], ["Harness", facts.harness]] : []) {
        if (!value) continue;
        const fact = document.createElement("span");
        fact.className = "agent-fact";
        const name = document.createElement("b");
        name.textContent = `${label} `;
        fact.append(name, document.createTextNode(value));
        meta.append(fact);
      }
      card.append(meta);
      const open = document.createElement("button");
      open.type = "button";
      open.className = "reviewer-result-open agent-details-open";
      open.setAttribute("aria-haspopup", "dialog");
      open.textContent = "View agent details";
      card.append(open);
      fragment.append(card);
    }
    flipAgentGrid(() => agentGrid.replaceChildren(fragment));
    agentSection.hidden = runAgents.size === 0;
    if (agentEmptyMessage) agentEmptyMessage.hidden = runAgents.size > 0;
    if (completedToggle) {
      completedToggle.hidden = activeCount === 0 || settled.length === 0;
      completedToggle.setAttribute("aria-expanded", String(completedAgentsExpanded));
      if (completedToggleLabel) completedToggleLabel.textContent = completedAgentsExpanded ? "Hide" : "Show";
      if (completedToggleCount) completedToggleCount.textContent = String(settled.length);
      if (completedTogglePlural) completedTogglePlural.hidden = settled.length === 1;
    }
  };

  completedToggle?.addEventListener("click", () => {
    completedAgentsExpanded = !completedAgentsExpanded;
    renderAgentGrid();
  });

  const refreshAgentDialogSummary = (agent) => {
    if (!agentDialogSummary || !agent || openAgentTask !== agent.task) return;
    const failure = agentFailures.get(agent.task);
    const facts = agentFacts.get(agent.task);
    const rows = [
      ["Status", agent.status],
      ["Latest event", agent.latest?.label || "—"],
      ["Last activity", formatRunTime(agent.latest?.time) || "—"],
      ["Tool calls", String(agent.toolCalls)],
      ["Events observed", String(agent.events.length)]
    ];
    if (facts) {
      if (facts.provider) rows.push(["Provider", facts.provider]);
      if (facts.model) rows.push(["Model", facts.model]);
      if (facts.harness) rows.push(["Harness", facts.harness]);
      if (facts.session_id) rows.push(["Session ID", facts.session_id]);
      if (facts.attempts) rows.push(["Attempts", String(facts.attempts)]);
      const retries = facts.retries || Math.max(0, (facts.attempts || 1) - 1);
      if (retries) {
        rows.push(["Retries", String(retries)]);
        rows.push(["Session continued", facts.session_reuse === "same" ? "yes — retries kept the same session" : "no — retries started a fresh session"]);
      }
    }
    const retryWait = agentRetryWait(agent);
    if (retryWait) rows.push(["Backoff", retryWait]);
    if (failure) {
      if (failure.code) rows.splice(rows.length - (failure.message ? 1 : 0), 0, ["Failure code", failure.code]);
      if (failure.message) rows.push(["Failure reason", failure.message]);
    }
    agentDialogSummary.replaceChildren(...rows.map(([term, value]) => {
      const row = document.createElement("div");
      const dt = document.createElement("dt");
      const dd = document.createElement("dd");
      dt.textContent = term;
      dd.textContent = value;
      row.append(dt, dd);
      return row;
    }));
  };

  const ingestRunEvent = (event) => {
    if (!event?.task || !agentSection) return;
    let agent = runAgents.get(event.task);
    if (!agent) {
      agent = {task: event.task, events: [], toolCalls: 0, status: "running", latest: null};
      runAgents.set(event.task, agent);
    }
    const payload = event.payload || {};
    if (payload.kind === "tool") agent.toolCalls++;
    const mappedStatus = runAgentStatusFor(event);
    if (mappedStatus) agent.status = mappedStatus;
    if (mappedStatus === "failed" && !agentFailures.has(event.task)) loadAgentFailures();
    const label = describeRunEvent(event);
    agent.latest = {label, time: event.committed_at || "", sequence: Number(event.sequence) || 0};
    const entry = {label, detail: payload.action && payload.kind !== "tool" ? humanizeRunText(payload.action) : "", time: event.committed_at || ""};
    agent.events.push(entry);
    while (agent.events.length > 500) agent.events.shift();
    renderAgentGrid();
    if (openAgentTask === agent.task) {
      if (agentDialogTimeline) appendAgentStreamRow(agentDialogTimeline, entry);
      refreshAgentDialogSummary(agent);
    }
  };

  const openRunAgent = (task) => {
    if (!agentDialog) return;
    const agent = runAgents.get(task);
    if (!agent) return;
    openAgentTask = task;
    const identity = runAgentIdentity(task);
    if (agentDialogTitle) agentDialogTitle.textContent = identity.title || task;
    if (agentDialogSubtitle) agentDialogSubtitle.textContent = `${identity.kind} · ${task}`;
    refreshAgentDialogSummary(agent);
    if (agentDialogTimeline) {
      agentDialogTimeline.replaceChildren();
      for (const entry of agent.events) appendAgentStreamRow(agentDialogTimeline, entry);
    }
    agentDialog.showModal();
  };

  agentGrid?.addEventListener("click", (event) => {
    const trigger = event.target.closest(".run-agent-card");
    if (trigger?.dataset.runAgent) openRunAgent(trigger.dataset.runAgent);
  });
  agentDialogClose?.addEventListener("click", () => {
    openAgentTask = "";
    agentDialog.close();
  });
  agentDialog?.addEventListener("close", () => { openAgentTask = ""; });
  agentDialog?.addEventListener("click", (event) => {
    if (event.target === agentDialog) agentDialog.close();
  });

  for (const item of durableTimeline?.querySelectorAll("[data-run-sequence]") || []) {
    const task = item.dataset.runTask;
    if (!task) continue;
    ingestRunEvent({
      sequence: Number(item.dataset.runSequence) || 0,
      type: item.dataset.runType,
      stage: item.dataset.runStage,
      task,
      committed_at: item.dataset.runTime,
      payload: {kind: item.dataset.runKind, tool: item.dataset.runTool}
    });
  }
  renderAgentGrid();
  runAgentsLive = true;

  const appendDurableEvent = (event) => {
    ingestRunEvent(event);
    if (!durableTimeline) return;
    durableTimeline.querySelector(":scope > .empty")?.remove();
    const item = document.createElement("li");
    item.dataset.runSequence = String(event.sequence);
    const heading = document.createElement("strong");
    heading.textContent = `${event.sequence} · ${event.type || "event"}`;
    item.append(heading);
    for (const value of [event.stage, event.task]) {
      if (!value) continue;
      const detail = document.createElement("span");
      detail.textContent = ` ${value}`;
      item.append(detail);
    }
    if (event.omission) {
      const omission = document.createElement("p");
      omission.textContent = `Omitted ${event.omission.count || 0} detail item(s): ${event.omission.reason || "bounded history"}`;
      item.append(omission);
    }
    durableTimeline.append(item);
    let pruned = false;
    while (durableTimeline.children.length > 500) {
      durableTimeline.firstElementChild?.remove();
      pruned = true;
    }
    if (pruned) setDurableLive("Live — the page retains the newest 500 rows; repository history is unchanged.");
  };
  const stopDurableFollow = () => {
    durableStream?.close();
    durableStream = null;
    if (durableReconnect) window.clearTimeout(durableReconnect);
    durableReconnect = null;
  };
  const preflightDurableRun = async (runID) => {
    const replay = await fetch(`/api/v1/runs/${encodeURIComponent(runID)}/events?after=${durableLast}`, {headers: {Accept: "application/json"}});
    if (replay.status === 409) {
      const problem = await replay.json().catch(() => ({}));
      setDurableLive(problem?.error?.code === "replay_gap" ? "History gap detected. Refresh the snapshot to resume from the oldest retained event." : "Cursor no longer matches the durable run. Refresh required.");
      return false;
    }
    if (!replay.ok) throw new Error("run replay unavailable");
    const body = await replay.json();
    for (const event of body?.data?.events || []) {
      if (Number(event.sequence) <= durableLast) continue;
      appendDurableEvent(event);
      durableLast = Number(event.sequence);
    }
    const lifecycle = body?.data?.run?.lifecycle;
    if (["succeeded", "failed", "cancelled", "timed_out", "interrupted", "cleanup_uncertain", "persistence_degraded"].includes(lifecycle)) {
      durableTerminal = true;
      setDurableLive(`Terminal: ${lifecycle}.`);
      return false;
    }
    return true;
  };
  const followDurableRun = async () => {
    if (!durableRun || !durableTimeline || document.hidden || durableTerminal || durableStream) return;
    const runID = durableRun.dataset.runId;
    try {
      if (!await preflightDurableRun(runID)) return;
    } catch (_) {
      setDurableLive("Run store unavailable; showing the last committed snapshot.");
      durableReconnect = window.setTimeout(followDurableRun, 1000);
      return;
    }
    setDurableLive("Connecting to committed run events…");
    durableStream = new EventSource(`/api/v1/runs/${encodeURIComponent(runID)}/events?after=${durableLast}`);
    durableStream.addEventListener("open", () => setDurableLive("Live — committed events only."));
    durableStream.addEventListener("run", (message) => {
      let event;
      try { event = JSON.parse(message.data); } catch (_) { return; }
      const sequence = Number(event.sequence);
      if (!Number.isSafeInteger(sequence) || sequence <= durableLast) return;
      appendDurableEvent(event);
      durableLast = sequence;
      if (event.type === "terminal") {
        durableTerminal = true;
        setDurableLive("Terminal event committed.");
        stopDurableFollow();
      }
    });
    durableStream.addEventListener("error", () => {
      durableStream?.close();
      durableStream = null;
      if (!durableTerminal && !document.hidden) {
        setDurableLive("Reconnecting from the last committed sequence…");
        durableReconnect = window.setTimeout(followDurableRun, 1000);
      }
    });
  };

  if (durableRun) {
    followDurableRun();
    document.addEventListener("visibilitychange", () => {
      if (document.hidden) {
        stopDurableFollow();
        if (!durableTerminal) setDurableLive("Observation paused while this tab is hidden.");
      } else {
        followDurableRun();
      }
    });
  }

  if (statusRoot) {
    follow({id: statusRoot.dataset.operationId, state: statusRoot.dataset.operationState});
  } else if (reviewStatus) {
    refreshReviewers();
    reviewTimer = setInterval(refreshReviewers, 2000);
  }

  window.addEventListener("pagehide", () => {
    if (stream) stream.close();
    stopDurableFollow();
    if (durableLiveTimer) window.clearTimeout(durableLiveTimer);
    if (reviewTimer) clearInterval(reviewTimer);
  });
})();
