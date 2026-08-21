(() => {
  const monitor = document.querySelector("[data-study-resources]");
  if (!monitor) return;
  const metric = (name) => monitor.querySelector(`[data-resource="${name}"]`);
  const bytes = (amount) => amount ? `${(amount / 1073741824).toFixed(amount >= 1073741824 ? 2 : 3)} GiB` : "0 GiB";
  const duration = (ms) => ms >= 60000 ? `${(ms / 60000).toFixed(1)} min` : `${(ms / 1000).toFixed(1)} s`;
  const points = (samples, field) => {
    if (!samples.length) return "";
    const max = Math.max(1, ...samples.map((sample) => sample[field] || 0));
    return samples.map((sample, index) => {
      const x = samples.length === 1 ? 0 : index * 720 / (samples.length - 1);
      return `${x},${176 - (sample[field] || 0) * 168 / max}`;
    }).join(" ");
  };
  const render = (history) => {
    const samples = history.samples || [];
    const latest = samples[samples.length - 1];
    monitor.querySelector("[data-resource-empty]").hidden = Boolean(latest);
    monitor.querySelector(".resource-chart").hidden = !latest;
    monitor.querySelector(".resource-processes").hidden = !latest;
    if (!latest) return;
    metric("available").textContent = bytes(latest.memory_available_bytes);
    metric("parent-rss").textContent = bytes(latest.process_rss_bytes);
    metric("child-rss").textContent = bytes(latest.child_rss_bytes);
    metric("swap").textContent = bytes(latest.process_swap_bytes);
    metric("workers").textContent = String(latest.child_process_count || 0);
    metric("parallelism").textContent = latest.effective_parallelism ? `${latest.effective_parallelism} / ${latest.requested_parallelism}` : "not throttled";
    monitor.querySelector('[data-resource-line="children"]').setAttribute("points", points(samples, "child_rss_bytes"));
    monitor.querySelector('[data-resource-line="parent"]').setAttribute("points", points(samples, "process_rss_bytes"));
    const rows = monitor.querySelector("[data-resource-processes]");
    rows.replaceChildren(...(latest.children || []).map((child) => {
      const row = document.createElement("tr");
      [child.pid, child.task_id || "unassigned", bytes(child.rss_bytes), bytes(child.swap_bytes), duration(child.cpu_time_ms), duration(child.elapsed_ms)].forEach((text) => {
        const cell = document.createElement("td");
        cell.textContent = text;
        row.appendChild(cell);
      });
      return row;
    }));
    const controls = samples.filter((sample) => sample.phase === "parallelism.throttled" || sample.phase === "parallelism.restored").slice(-8).reverse();
    const list = monitor.querySelector("[data-resource-events]");
    const events = controls.length ? controls : [{phase: "No throttle events recorded."}];
    list.replaceChildren(...events.map((event) => {
      const item = document.createElement("li");
      item.textContent = event.timestamp ? `${new Date(event.timestamp).toLocaleTimeString()}  ${event.phase.replace("parallelism.", "")} to ${event.effective_parallelism}/${event.requested_parallelism}, ${bytes(event.memory_available_bytes)} available` : event.phase;
      return item;
    }));
    monitor.querySelector("[data-resource-state]").textContent = `Updated ${new Date(latest.timestamp).toLocaleTimeString()}`;
  };
  const load = async () => {
    try {
      const response = await fetch(monitor.dataset.studyResources, {headers: {Accept: "application/json"}});
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const payload = await response.json();
      render(payload.data || payload);
    } catch (_) {
      monitor.querySelector("[data-resource-state]").textContent = "Diagnostics unavailable";
    }
  };
  load();
  window.setInterval(load, 5000);
})();
