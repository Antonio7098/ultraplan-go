(() => {
  "use strict";
  const stableEvents = Object.freeze(["snapshot", "progress", "warning", "finding", "artifact", "cancel_requested", "recovery_required", "terminal"]);
  window.UltraPlanSSE = Object.freeze({
    stableEvents,
    closeOnAbort(stream, signal = window.UltraPlan?.signal) {
      if (!signal) return;
      if (signal.aborted) stream.close();
      else signal.addEventListener("abort", () => stream.close(), {once: true});
    }
  });
})();
