(() => {
  "use strict";

  const refreshStatus = document.getElementById("refresh-status");
  for (const link of document.querySelectorAll(".refresh-link")) {
    link.addEventListener("click", () => {
      if (refreshStatus) refreshStatus.textContent = "Refreshing current workspace state.";
    });
  }

  const forms = [...document.querySelectorAll(".operation-form")];
  if (forms.length === 0) return;

  const csrf = document.querySelector('meta[name="ultraplan-csrf"]')?.content || "";
  const confirmation = document.getElementById("operation-confirmation");
  const live = document.getElementById("operation-live");
  const timeline = document.getElementById("operation-timeline");
  const cancelButton = document.getElementById("operation-cancel");
  let stream = null;
  let currentOperationID = "";
  let lastSequence = 0;

  function specification(form) {
    const scope = {};
    if (form.dataset.project) scope.project = form.dataset.project;
    if (form.dataset.sprint) scope.sprint = form.dataset.sprint;
    if (form.dataset.study) scope.study = form.dataset.study;
    const options = {};
    if (form.dataset.stage) options.to_stage = form.dataset.stage;
    if (form.dataset.parallelism) options.parallelism = Number(form.dataset.parallelism);
    return {kind: form.dataset.operationKind, scope, options};
  }

  async function command(path, payload, method = "POST") {
    const response = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf},
      body: payload === null ? undefined : JSON.stringify(payload)
    });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error?.message || `Request failed (${response.status})`);
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
    item.textContent = `${name}: ${payload.message || payload.state || payload.reason || "update"}`;
    timeline.append(item);
    while (timeline.children.length > 100) timeline.firstElementChild.remove();
  }

  function follow(operation) {
    if (stream) stream.close();
    currentOperationID = operation.id;
    lastSequence = 0;
    if (cancelButton) cancelButton.hidden = false;
    announce(`Operation ${operation.id} ${operation.state}.`);
    stream = new EventSource(`/api/v1/operations/${encodeURIComponent(operation.id)}/events`);
    for (const name of ["snapshot", "progress", "warning", "finding", "artifact", "cancel_requested", "recovery_required", "terminal"]) {
      stream.addEventListener(name, (message) => {
        let event;
        try { event = JSON.parse(message.data); } catch { return; }
        appendEvent(name, event);
        if (name === "recovery_required") announce("Some transient progress expired. Refresh durable status for complete truth.");
        if (name === "terminal") {
          stream.close();
          stream = null;
          if (cancelButton) cancelButton.hidden = true;
          announce(`Operation ${event.payload?.state || "finished"}.`);
        }
      });
    }
    stream.onerror = () => announce("Progress connection was interrupted. Reconnecting or refresh durable status.", true);
  }

  for (const form of forms) {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        const operation = specification(form);
        announce("Preparing normalized operation scope.");
        const prepared = await command("/api/v1/operations/prepare", {operation});
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
            confirmation.hidden = true;
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

  cancelButton?.addEventListener("click", async () => {
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
  });

  window.addEventListener("pagehide", () => {
    if (stream) stream.close();
  });
})();
