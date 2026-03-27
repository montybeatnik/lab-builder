(function () {
  if (window.__labLiveInit) return;
  window.__labLiveInit = true;

  function $(id) { return document.getElementById(id); }
  const svgNS = 'http://www.w3.org/2000/svg';
  const liveView = {
    width: 1400,
    height: 760,
    zoom: 1,
    centerX: 700,
    centerY: 380
  };
  let pollTimer = null;
  let panDrag = null;
  let lastLiveData = null;

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
    const width = liveView.width;
    const height = liveView.height;
    const spineXs = spreadX(spine.length, width, 120);
    const leafXs = spreadX(leaf.length, width, 120);
    spine.forEach((n, i) => { layout[n] = { x: spineXs[i], y: 150, role: 'spine' }; });
    leaf.forEach((n, i) => { layout[n] = { x: leafXs[i], y: 540, role: 'leaf' }; });
    const edgeXs = spreadX(edge.length, width, 140);
    edge.forEach((n, i) => { layout[n] = { x: edgeXs[i], y: 680, role: 'edge' }; });
    const restXs = spreadX(rest.length, width, 160);
    rest.forEach((n, i) => { layout[n] = { x: restXs[i], y: 340, role: 'mesh' }; });
    return layout;
  }

  function clamp(v, min, max) {
    if (v < min) return min;
    if (v > max) return max;
    return v;
  }

  function clampDelta(v) {
    return clamp(v, -80, 80);
  }

  function panByWheel(dx, dy) {
    // Map-like touchpad pan: content follows finger motion and is smoothed.
    const factor = 0.65;
    panBy(-(dx * factor) / liveView.zoom, -(dy * factor) / liveView.zoom);
  }

  function applyZoomViewBox() {
    const svg = $('liveGraph');
    if (!svg) return;
    const zoom = clamp(liveView.zoom, 0.6, 3);
    liveView.zoom = zoom;
    const w = liveView.width / zoom;
    const h = liveView.height / zoom;
    const minCx = w / 2;
    const maxCx = liveView.width - (w / 2);
    const minCy = h / 2;
    const maxCy = liveView.height - (h / 2);
    liveView.centerX = clamp(liveView.centerX, minCx, maxCx);
    liveView.centerY = clamp(liveView.centerY, minCy, maxCy);
    const x = liveView.centerX - (w / 2);
    const y = liveView.centerY - (h / 2);
    svg.setAttribute('viewBox', `${x} ${y} ${w} ${h}`);
    const lbl = $('liveZoomLabel');
    if (lbl) lbl.textContent = `${Math.round(zoom * 100)}%`;
  }

  function setZoom(zoom) {
    liveView.zoom = zoom;
    applyZoomViewBox();
  }

  function panBy(dx, dy) {
    liveView.centerX += dx;
    liveView.centerY += dy;
    applyZoomViewBox();
  }

  function renderLiveGraph(data) {
    const svg = $('liveGraph');
    if (!svg) return;
    svg.innerHTML = '';
    svg.classList.add('future-mode');
    const wrap = svg.closest('.graph-wrap');
    if (wrap) wrap.classList.add('future-wrap');
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

    const peerings = data.peerings || [];
    const showPeerings = $('liveShowPeerings') && $('liveShowPeerings').checked;
    if (showPeerings) {
      peerings.forEach(peer => {
        const a = layout[peer.a];
        const b = layout[peer.b];
        if (!a || !b) return;
        const line = document.createElementNS(svgNS, 'line');
        line.setAttribute('x1', a.x);
        line.setAttribute('y1', a.y - 8);
        line.setAttribute('x2', b.x);
        line.setAttribute('y2', b.y - 8);
        line.setAttribute('class', `live-peering ${peer.state === 'up' ? 'live-peering-up' : 'live-peering-down'}`);
        const title = document.createElementNS(svgNS, 'title');
        title.textContent = `${peer.a} <-> ${peer.b} ${peer.afiSafi} (${peer.detail || peer.state})`;
        line.appendChild(title);
        svg.appendChild(line);

        const midX = (a.x + b.x) / 2;
        const midY = (a.y + b.y) / 2 - 12;
        const label = document.createElementNS(svgNS, 'text');
        label.setAttribute('x', midX);
        label.setAttribute('y', midY);
        label.setAttribute('text-anchor', 'middle');
        label.setAttribute('class', 'live-peering-label');
        label.textContent = peer.afiSafi;
        svg.appendChild(label);
      });
    }

    names.forEach(name => {
      const pos = layout[name];
      if (!pos) return;
      const group = document.createElementNS(svgNS, 'g');
      const isServer = pos.role === 'edge';
      group.setAttribute('class', `node ${pos.role} ${isServer ? 'node-server' : 'node-switch'}`);
      const rect = document.createElementNS(svgNS, 'rect');
      const width = isServer ? 52 : 60;
      const height = isServer ? 26 : 22;
      rect.setAttribute('x', pos.x - (width / 2));
      rect.setAttribute('y', pos.y - (height / 2));
      rect.setAttribute('width', width);
      rect.setAttribute('height', height);
      rect.setAttribute('rx', isServer ? 4 : 6);
      rect.setAttribute('class', isServer ? 'server-chassis' : 'switch-chassis');
      group.appendChild(rect);

      if (isServer) {
        for (let i = 0; i < 3; i++) {
          const bay = document.createElementNS(svgNS, 'line');
          const y = pos.y - 7 + (i * 7);
          bay.setAttribute('x1', pos.x - 18);
          bay.setAttribute('y1', y);
          bay.setAttribute('x2', pos.x + 14);
          bay.setAttribute('y2', y);
          bay.setAttribute('class', 'server-bay');
          group.appendChild(bay);
        }
        const led = document.createElementNS(svgNS, 'circle');
        led.setAttribute('cx', pos.x + 20);
        led.setAttribute('cy', pos.y);
        led.setAttribute('r', 2.6);
        led.setAttribute('class', 'server-led');
        group.appendChild(led);
      } else {
        const top = document.createElementNS(svgNS, 'line');
        top.setAttribute('x1', pos.x - 24);
        top.setAttribute('y1', pos.y - 5);
        top.setAttribute('x2', pos.x + 24);
        top.setAttribute('y2', pos.y - 5);
        top.setAttribute('class', 'switch-topline');
        group.appendChild(top);
        for (let i = 0; i < 6; i++) {
          const port = document.createElementNS(svgNS, 'circle');
          port.setAttribute('cx', pos.x - 20 + (i * 8));
          port.setAttribute('cy', pos.y + 4);
          port.setAttribute('r', 1.6);
          port.setAttribute('class', 'switch-port');
          group.appendChild(port);
        }
      }
      const label = document.createElementNS(svgNS, 'text');
      label.setAttribute('x', pos.x);
      label.setAttribute('y', pos.y + 34);
      label.setAttribute('text-anchor', 'middle');
      label.textContent = name;
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
      sudo: $('liveUseSudo').value === 'true',
      showPeerings: $('liveShowPeerings') ? $('liveShowPeerings').checked : false
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
      style: 'futuristic',
      peerings: {
        enabled: $('liveShowPeerings').checked,
        count: (data.peerings || []).length,
        sample: (data.peerings || []).slice(0, 10)
      }
    }, null, 2);
    renderLiveGraph(data);
    lastLiveData = data;
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
    applyZoomViewBox();
    loadLabs();
    $('liveStartBtn').addEventListener('click', startPolling);
    $('liveStopBtn').addEventListener('click', stopPolling);
    $('liveLabSelect').addEventListener('change', () => {
      if (pollTimer) pollLive();
    });
    $('liveShowPeerings').addEventListener('change', () => {
      pollLive();
    });
    $('liveZoomInBtn').addEventListener('click', () => setZoom(liveView.zoom + 0.2));
    $('liveZoomOutBtn').addEventListener('click', () => setZoom(liveView.zoom - 0.2));
    $('liveZoomResetBtn').addEventListener('click', () => {
      liveView.centerX = liveView.width / 2;
      liveView.centerY = liveView.height / 2;
      setZoom(1);
    });
    $('livePanLeftBtn').addEventListener('click', () => panBy(-80 / liveView.zoom, 0));
    $('livePanRightBtn').addEventListener('click', () => panBy(80 / liveView.zoom, 0));
    $('livePanUpBtn').addEventListener('click', () => panBy(0, -80 / liveView.zoom));
    $('livePanDownBtn').addEventListener('click', () => panBy(0, 80 / liveView.zoom));
    $('liveGraph').addEventListener('wheel', e => {
      e.preventDefault();
      // Touchpad/scroll-wheel UX:
      // - Two-finger scroll pans the canvas.
      // - Pinch gesture (Ctrl+wheel on macOS) zooms.
      if (e.ctrlKey) {
        const delta = e.deltaY > 0 ? -0.08 : 0.08;
        setZoom(liveView.zoom + delta);
        return;
      }
      const dx = clampDelta(e.deltaX);
      const dy = clampDelta(e.deltaY);
      panByWheel(dx, dy);
    }, { passive: false });
    $('liveGraph').addEventListener('mousedown', e => {
      panDrag = { x: e.clientX, y: e.clientY };
    });
    window.addEventListener('mousemove', e => {
      if (!panDrag) return;
      const dx = e.clientX - panDrag.x;
      const dy = e.clientY - panDrag.y;
      panDrag = { x: e.clientX, y: e.clientY };
      panBy(-(dx / liveView.zoom), -(dy / liveView.zoom));
    });
    window.addEventListener('mouseup', () => {
      panDrag = null;
    });
  });
})();
