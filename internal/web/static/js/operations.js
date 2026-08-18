(() => {
  "use strict";
  const runtime = window.UltraPlan;
  if (!runtime) return;
  window.UltraPlanOperations = Object.freeze({
    async command(path, payload, method = "POST") {
      const csrf = document.querySelector('meta[name="ultraplan-csrf"]')?.content || "";
      const response = await fetch(path, {
        method,
        credentials: "same-origin",
        headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf},
        body: payload === null ? undefined : JSON.stringify(payload),
        signal: runtime.signal
      });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error?.message || `Request failed (${response.status})`);
      return body.data;
    }
  });
})();
