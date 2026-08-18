(() => {
  "use strict";
  const controller = new AbortController();
  window.UltraPlan = Object.freeze({controller, signal: controller.signal});
  window.addEventListener("pagehide", () => controller.abort("navigation"), {once: true});
})();
