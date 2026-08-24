(() => {
  "use strict";
  const runtime = window.UltraPlan;
  if (!runtime) return;
  window.UltraPlanOperations = Object.freeze({
    async command(path, payload, method = "POST") {
      const csrfMeta = document.querySelector('meta[name="ultraplan-csrf"]');
      let csrf = csrfMeta?.content || "";
      for (let attempt = 0; attempt < 2; attempt += 1) {
        const response = await fetch(path, {
          method,
          credentials: "same-origin",
          headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf},
          body: payload === null ? undefined : JSON.stringify(payload),
          signal: runtime.signal
        });
        const body = await response.json();
        const code = body.error?.code;
        const replacementCSRF = response.headers.get("X-CSRF-Token");
        if (attempt === 0 && response.status === 403 && replacementCSRF && (code === "session_required" || code === "csrf_failed")) {
          csrf = replacementCSRF;
          if (csrfMeta) csrfMeta.content = csrf;
          continue;
        }
        if (!response.ok) {
          const parts = [body.error?.message, body.error?.details?.reason, body.error?.details?.guidance].filter(Boolean);
          throw new Error(parts.join(" ") || `Request failed (${response.status})`);
        }
        return body.data;
      }
    }
  });
})();
