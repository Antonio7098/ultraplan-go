(() => {
  const panel = document.querySelector("[data-run-timeline]");
  if (!panel) return;
  const state = panel.querySelector("[data-timeline-state]");
  const empty = panel.querySelector("[data-timeline-empty]");
  const chartWrap = panel.querySelector("[data-timeline-chart]");
  const windowSelect = panel.querySelector("[data-timeline-window]");
  const limitSelect = panel.querySelector("[data-timeline-limit]");
  const width = 900;
  const gutter = 176;
  const rightPad = 14;
  const rowHeight = 30;
  const topPad = 12;
  const axisHeight = 26;
  const spikeMax = rowHeight * 0.58;
  const lifecycleClass = (lifecycle) => {
    if (lifecycle === "succeeded") return "ok";
    if (["accepted", "queued", "running", "cancelling"].includes(lifecycle)) return "info";
    if (["failed", "cancelled", "timed_out", "interrupted", "cleanup_uncertain", "persistence_degraded"].includes(lifecycle)) return "error";
    return "warn";
  };
  const shortID = (id) => (id.length > 18 ? `${id.slice(0, 15)}…` : id);
  const clock = (iso) => new Date(iso).toLocaleTimeString([], {hour: "2-digit", minute: "2-digit"});
  const span = (ms) => {
    if (ms >= 86400000) return `${(ms / 3600000).toFixed(1)} h`;
    if (ms >= 60000) return `${(ms / 60000).toFixed(1)} min`;
    return `${(ms / 1000).toFixed(1)} s`;
  };
  const el = (name, attrs) => {
    const node = document.createElementNS("http://www.w3.org/2000/svg", name);
    for (const key of Object.keys(attrs || {})) node.setAttribute(key, attrs[key]);
    return node;
  };
  const label = (text, x, y, anchor) => {
    const node = el("text", {x, y, "text-anchor": anchor || "end"});
    node.textContent = text;
    return node;
  };
  const spikes = (run, x0, x1, baseline) => {
    const stamps = (run.tool_events || []).map((value) => Date.parse(value)).filter((time) => Number.isFinite(time));
    const points = [[x0, baseline]];
    if (stamps.length && x1 > x0) {
      const buckets = Math.max(8, Math.min(64, Math.round((x1 - x0) / 6)));
      const counts = new Array(buckets).fill(0);
      let peak = 0;
      for (const time of stamps) {
        const index = Math.min(buckets - 1, Math.max(0, Math.floor((time - run.__start) / (run.__end - run.__start || 1) * buckets)));
        counts[index] += 1;
        if (counts[index] > peak) peak = counts[index];
      }
      for (let index = 0; index < buckets; index += 1) {
        if (!counts[index]) continue;
        const cx = x0 + (index + 0.5) * (x1 - x0) / buckets;
        points.push([cx, baseline - counts[index] / peak * spikeMax]);
        points.push([cx + (x1 - x0) / buckets / 2 - 0.01, baseline]);
      }
    }
    points.push([Math.max(points[points.length - 1][0], x1), baseline]);
    return points.map(([px, py]) => `${px.toFixed(1)},${py.toFixed(1)}`).join(" ");
  };
  const render = (payload) => {
    const runs = payload.runs || [];
    state.hidden = false;
    empty.hidden = Boolean(runs.length);
    chartWrap.hidden = !runs.length;
    if (!runs.length) {
      state.textContent = "No runs in window";
      return;
    }
    const windowStart = Date.parse(payload.window_start);
    const windowEnd = Date.parse(payload.window_end);
    const domain = Math.max(1, windowEnd - windowStart);
    const x = (time) => gutter + (Math.min(Math.max(time, windowStart), windowEnd) - windowStart) / domain * (width - gutter - rightPad);
    const height = topPad + runs.length * rowHeight + axisHeight;
    const svg = el("svg", {viewBox: `0 0 ${width} ${height}`, role: "img", "aria-label": "Run history timeline with tool activity spikes"});
    for (let tick = 0; tick <= 4; tick += 1) {
      const time = windowStart + tick * domain / 4;
      const tx = x(time);
      svg.appendChild(el("line", {x1: tx, y1: topPad, x2: tx, y2: height - axisHeight, class: "run-timeline-grid"}));
      svg.appendChild(label(tick === 4 ? clock(new Date(windowEnd).toISOString()) : clock(new Date(time).toISOString()), tx, height - 8, tick === 0 ? "start" : tick === 4 ? "end" : "middle"));
    }
    runs.forEach((run, index) => {
      const start = Date.parse(run.started_at);
      const end = run.active ? windowEnd : Math.max(Date.parse(run.ended_at) || windowEnd, start);
      run.__start = start;
      run.__end = end;
      const baseline = topPad + index * rowHeight + rowHeight * 0.66;
      const x0 = Math.max(x(start), gutter);
      const x1 = Math.max(x(end), x0 + 2);
      const group = el("g", {class: `run-timeline-row run-lifecycle-${lifecycleClass(run.lifecycle)}`});
      const link = el("a", {href: `/runs/${encodeURIComponent(run.run_id)}`});
      const title = el("title");
      title.textContent = `${run.run_id}\n${run.target || ""}\n${run.lifecycle} · ${span(Math.max(0, end - start))}${run.active ? " · active" : ""}\n${(run.tool_events || []).length} tool event(s)${run.tool_events_sampled ? " (sampled)" : ""}`;
      group.appendChild(label(shortID(run.run_id), gutter - 10, baseline + 4));
      const statusText = label(run.lifecycle, width - rightPad, baseline + 4, "end");
      statusText.setAttribute("class", "run-timeline-status");
      link.appendChild(title);
      link.appendChild(el("line", {x1: x0, y1: baseline, x2: x1, y2: baseline, class: "run-timeline-base"}));
      link.appendChild(el("polyline", {points: spikes(run, x0, x1, baseline), class: "run-timeline-activity"}));
      if (run.active) link.appendChild(el("circle", {cx: x1, cy: baseline, r: 3, class: "run-timeline-live"}));
      group.appendChild(link);
      group.appendChild(statusText);
      svg.appendChild(group);
    });
    chartWrap.replaceChildren(svg);
    const activeCount = runs.filter((run) => run.active).length;
    state.textContent = `Updated ${new Date().toLocaleTimeString()} · ${runs.length} run(s)${activeCount ? `, ${activeCount} active` : ""}`;
  };
  const load = async () => {
    const params = new URLSearchParams();
    params.set(panel.dataset.timelineScope, panel.dataset.timelineScope === "study" ? panel.dataset.timelineStudy : panel.dataset.timelineSprint);
    params.set("window", windowSelect.value);
    params.set("limit", limitSelect.value);
    try {
      const response = await fetch(`/api/v1/timeline?${params}`, {headers: {Accept: "application/json"}});
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      render((await response.json()).data || {});
    } catch (_) {
      state.textContent = "Run history unavailable";
    }
  };
  windowSelect.addEventListener("change", load);
  limitSelect.addEventListener("change", load);
  load();
  window.setInterval(() => {
    if (!document.hidden) load();
  }, 15000);
})();
