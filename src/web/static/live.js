(function () {
  if (window.__labLiveInit) return;
  window.__labLiveInit = true;

  function $(id) { return document.getElementById(id); }
  const svgNS = 'http://www.w3.org/2000/svg';
  let pollTimer = null;

  function setStatus(text, cls) {
    const status = $('liveStatus');
    if (!status) return;
    status.textContent = text;
    status.className = `status ${cls || 'status-idle'}`;
  }

  async function loadLabs() {
    const select = $('liveLabSelect');
    if (!select) return;
    const res = await fetch('/labs');
    const data = await res.json().catch(() => ({ ok: false }));
    if (!data.ok || !data.labs || data.labs.length === 0) {
      select.innerHTML = '<option value="">No labs found</option>';
      return;
    }
    select.innerHTML = '<option value="">Select a lab...</option>';
    data.labs.forEach(lab => {
      const opt = document.createElement('option');
      opt.value = lab.name;
      opt.textContent = `${lab.name} (${lab.path})`;
      select.appendChild(opt);
    });
  }

  function spreadX(count, width, margin) {
    if (count <= 1) return [width / 2];
    const usable = width - margin * 2;
    return Array.from({ length: count }, (_, i) => margin + (usable * i) / (count - 1));
  }

  function layoutNodes(names, nodes) {
    const layout = {};
    const spine = nodes.filter(n => n.role === 'spine' || n.role === 'hub').map(n => n.name);
    const leaf = nodes.filter(n => n.role === 'leaf' || n.role === 'spoke').map(n => n.name);
    const edge = names.filter(n => n.startsWith('edge'));
    const rest = names.filter(n => !spine.includes(n) && !leaf.includes(n) && !edge.includes(n));
    const width = 1000;
    const height = 520;
    const spineXs = spreadX(spine.length, width, 120);
    const leafXs = spreadX(leaf.length, width, 120);
    spine.forEach((n, i) => { layout[n] = { x: spineXs[i], y: 120, role: 'spine' }; });
    leaf.forEach((n, i) => { layout[n] = { x: leafXs[i], y: 380, role: 'leaf' }; });
    const edgeXs = spreadX(edge.length, width, 140);
    edge.forEach((n, i) => { layout[n] = { x: edgeXs[i], y: 470, role: 'edge' }; });
    const restXs = spreadX(rest.length, width, 160);
    rest.forEach((n, i) => { layout[n] = { x: restXs[i], y: 250, role: 'mesh' }; });
    return layout;
  }

  function renderLiveGraph(data) {
    const svg = $('liveGraph');
    if (!svg) return;
    svg.innerHTML = '';
    const nodes = data.nodes || [];
    const links = data.links || [];
    const names = nodes.map(n => n.name);
    links.forEach(link => {
      if (link.a && !names.includes(link.a)) names.push(link.a);
      if (link.b && !names.includes(link.b)) names.push(link.b);
    });
    const layout = layoutNodes(names, nodes);

    links.forEach(link => {
      const a = layout[link.a];
      const b = layout[link.b];
      if (!a || !b) return;
      const line = document.createElementNS(svgNS, 'line');
      line.setAttribute('x1', a.x);
      line.setAttribute('y1', a.y);
      line.setAttribute('x2', b.x);
      line.setAttribute('y2', b.y);
      const state = (link.state || 'unknown').toLowerCase();
      line.setAttribute('class', `edge live-link ${state === 'up' ? 'live-link-up' : (state === 'down' ? 'live-link-down' : 'live-link-unknown')}`);
      svg.appendChild(line);
    });

    names.forEach(name => {
      const pos = layout[name];
      if (!pos) return;
      const group = document.createElementNS(svgNS, 'g');
      group.setAttribute('class', `node ${pos.role}`);
      const circle = document.createElementNS(svgNS, 'circle');
      circle.setAttribute('cx', pos.x);
      circle.setAttribute('cy', pos.y);
      circle.setAttribute('r', 18);
      const label = document.createElementNS(svgNS, 'text');
      label.setAttribute('x', pos.x);
      label.setAttribute('y', pos.y + 34);
      label.setAttribute('text-anchor', 'middle');
      label.textContent = name;
      group.appendChild(circle);
      group.appendChild(label);
      svg.appendChild(group);
    });
  }

  async function pollLive() {
    const labName = $('liveLabSelect') ? $('liveLabSelect').value : '';
    if (!labName) {
      setStatus('Select a lab', 'status-idle');
      return;
    }
    setStatus('Polling telemetry...', 'status-pending');
    const payload = {
      labName,
      sudo: $('liveUseSudo').value === 'true'
    };
    const res = await fetch('/topology/live', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      setStatus('Polling failed', 'status-fail');
      $('liveDetails').textContent = data.error || 'live polling failed';
      return;
    }
    setStatus('Live telemetry active', 'status-pass');
    $('livePolledAt').textContent = data.polledAt ? `Polled: ${data.polledAt}` : '';
    $('liveDetails').textContent = JSON.stringify({
      labName: data.labName,
      linkSummary: data.summary,
      showPeerings: $('liveShowPeerings').checked ? 'staged-next' : 'disabled'
    }, null, 2);
    renderLiveGraph(data);
  }

  function startPolling() {
    stopPolling();
    pollLive();
    const sec = parseInt($('livePollSec').value, 10);
    const interval = Number.isFinite(sec) && sec >= 1 ? sec : 3;
    pollTimer = setInterval(pollLive, interval * 1000);
  }

  function stopPolling() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = null;
    setStatus('Polling stopped', 'status-idle');
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (!$('liveLabSelect')) return;
    loadLabs();
    $('liveStartBtn').addEventListener('click', startPolling);
    $('liveStopBtn').addEventListener('click', stopPolling);
    $('liveLabSelect').addEventListener('change', () => {
      if (pollTimer) pollLive();
    });
  });
})();
