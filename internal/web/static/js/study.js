(function () {
  "use strict";

  var browser = document.querySelector("[data-dimensions-browser]");
  if (browser) {
    var input = browser.querySelector("[data-dimension-search]");
    var cards = Array.prototype.slice.call(browser.querySelectorAll("[data-dimension-card]"));
    var count = browser.querySelector("[data-dimension-count]");
    var empty = browser.querySelector("[data-dimension-empty]");
    if (!input || cards.length === 0) {
      if (count) count.textContent = cards.length + " dimensions";
    } else {
      var apply = function () {
        var query = input.value.trim().toLowerCase();
        var visible = 0;
        cards.forEach(function (card) {
          var match = query === "" || card.textContent.toLowerCase().indexOf(query) !== -1;
          card.hidden = !match;
          if (match) visible++;
        });
        if (count) count.textContent = visible + " / " + cards.length + " shown";
        if (empty) empty.hidden = visible !== 0;
        var list = empty ? empty.previousElementSibling : null;
        if (list && list.tagName === "UL") list.hidden = visible === 0;
      };
      input.addEventListener("input", apply);
      apply();
    }
  }

  var tablist = document.querySelector("[data-report-tabs]");
  if (tablist) {
    var tabs = Array.prototype.slice.call(tablist.querySelectorAll("[role='tab']"));
    var panels = tabs.map(function (tab) { return document.getElementById(tab.getAttribute("aria-controls")); });
    var select = function (tab, focus) {
      tabs.forEach(function (item, index) {
        var active = item === tab;
        item.setAttribute("aria-selected", active ? "true" : "false");
        item.tabIndex = active ? 0 : -1;
        if (panels[index]) panels[index].hidden = !active;
      });
      if (focus) tab.focus();
    };
    tabs.forEach(function (tab, index) {
      tab.addEventListener("click", function () { select(tab, false); });
      tab.addEventListener("keydown", function (event) {
        var delta = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
        if (!delta) return;
        event.preventDefault();
        select(tabs[(index + delta + tabs.length) % tabs.length], true);
      });
    });
  }
})();
