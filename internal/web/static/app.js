(() => {
  "use strict";
  const status = document.getElementById("refresh-status");
  for (const link of document.querySelectorAll(".refresh-link")) {
    link.addEventListener("click", () => {
      if (status) status.textContent = "Refreshing current workspace state.";
    });
  }
})();
