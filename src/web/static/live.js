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
  let trafficOverlay = null;
  let trafficOverlayTimer = null;
  const liveTraffic = {
    edges: []
  };
  const liveTerminal = {
    node: '',
    socket: null,
    xterm: null,
    fitAddon: null
  };

  function setStatus(text, cls) {
    const status = $('liveStatus');
    if (!status) return;
    status.textContent = text;
    status.className = `status ${cls || 'status-idle'}`;
  }

  function setTrafficStatus(text) {
    const el = $('liveTrafficStatus');
    if (!el) return;
    el.textContent = text;
  }

  function ensureTerminalVisible() {
    const panel = $('liveConsolePanel');
    if (!panel || panel.hidden) return;
    const rect = panel.getBoundingClientRect();
    const viewportH = window.innerHeight || document.documentElement.clientHeight || 800;
    if (rect.bottom > viewportH - 16) {
      window.scrollBy({ top: rect.bottom - (viewportH - 16), behavior: 'smooth' });
    }
  }

  function initLiveXTerm() {
    const host = $('liveTerminalScreen');
    if (!host) return;
    if (liveTerminal.xterm) {
      liveTerminal.xterm.dispose();
      liveTerminal.xterm = null;
    }
    if (typeof window.Terminal !== 'function') {
      host.textContent = 'Terminal UI failed to load (xterm.js missing).';
      return;
    }
    liveTerminal.xterm = new window.Terminal({
      cursorBlink: true,
      fontSize: 13,
      lineHeight: 1.2,
      convertEol: true,
      theme: {
        background: '#050b19',
        foreground: '#d1fae5',
        cursor: '#7dd3fc'
      }
    });
    liveTerminal.fitAddon = null;
    if (window.FitAddon && typeof window.FitAddon.FitAddon === 'function') {
      liveTerminal.fitAddon = new window.FitAddon.FitAddon();
      liveTerminal.xterm.loadAddon(liveTerminal.fitAddon);
    }
    host.innerHTML = '';
    liveTerminal.xterm.open(host);
    if (liveTerminal.fitAddon) liveTerminal.fitAddon.fit();
    liveTerminal.xterm.onData(data => {
      if (!liveTerminal.socket || liveTerminal.socket.readyState !== WebSocket.OPEN) return;
      liveTerminal.socket.send(JSON.stringify({ type: 'input', data }));
    });
    liveTerminal.xterm.onResize(size => {
      if (!liveTerminal.socket || liveTerminal.socket.readyState !== WebSocket.OPEN) return;
      liveTerminal.socket.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
    });
  }

  async function closeLiveTerminalSession() {
    if (liveTerminal.socket) {
      try {
        liveTerminal.socket.send(JSON.stringify({ type: 'close' }));
      } catch {}
      liveTerminal.socket.close();
      liveTerminal.socket = null;
    }
  }

  async function startLiveTerminalSession(nodeName) {
    const labName = $('liveLabSelect') ? $('liveLabSelect').value : '';
    if (!labName || !nodeName) return;
    liveTerminal.node = nodeName;
    await closeLiveTerminalSession();
    initLiveXTerm();
    const panel = $('liveConsolePanel');
    const nodeLabel = $('liveConsoleNode');
    if (panel) panel.hidden = false;
    if (nodeLabel) nodeLabel.textContent = `Selected node: ${nodeName}`;
    ensureTerminalVisible();

    const qs = new URLSearchParams({
      labName,
      nodeName,
      sudo: String($('liveUseSudo').value === 'true'),
      timeoutSec: '30'
    });
    const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const wsURL = `${scheme}://${window.location.host}/walkthroughs/terminal/ws?${qs.toString()}`;
    const ws = new WebSocket(wsURL);
    liveTerminal.socket = ws;
    ws.addEventListener('open', () => {
      if (liveTerminal.fitAddon) liveTerminal.fitAddon.fit();
      if (liveTerminal.xterm) {
        liveTerminal.xterm.focus();
        liveTerminal.xterm.writeln('\x1b[90m[interactive terminal ready]\x1b[0m');
        ws.send(JSON.stringify({ type: 'resize', cols: liveTerminal.xterm.cols || 80, rows: liveTerminal.xterm.rows || 24 }));
      }
      setStatus(`Terminal connected: ${nodeName}`, 'status-pass');
    });
    ws.addEventListener('message', ev => {
      let msg = null;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        if (liveTerminal.xterm) liveTerminal.xterm.write(String(ev.data));
        return;
      }
      if (msg.type === 'output' && liveTerminal.xterm) {
        const out = String(msg.data || '');
        liveTerminal.xterm.write(out);
        if (out.includes('\u001b[6n') || out.includes('[6n')) {
          if (liveTerminal.socket && liveTerminal.socket.readyState === WebSocket.OPEN) {
            liveTerminal.socket.send(JSON.stringify({ type: 'input', data: '\u001b[1;1R' }));
          }
        }
      } else if (msg.type === 'error' && liveTerminal.xterm) {
        liveTerminal.xterm.writeln(`\n[error] ${msg.data || 'terminal error'}`);
      }
    });
    ws.addEventListener('close', () => {
      if (liveTerminal.socket === ws) liveTerminal.socket = null;
      if (liveTerminal.xterm) liveTerminal.xterm.writeln('\n[session closed]');
    });
    ws.addEventListener('error', () => {
      setStatus('Terminal connection failed', 'status-fail');
    });
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

  async function loadTrafficEdgesFromPlan(labName) {
    if (!labName) {
      syncTrafficSelectors([], true);
      setTrafficStatus('Select source/target edge nodes.');
      return;
    }
    const res = await fetch(`/labplan?name=${encodeURIComponent(labName)}`);
    const data = await res.json().catch(() => ({ ok: false }));
    if (!data.ok) {
      // Keep existing options if plan lookup fails; live telemetry can still populate these.
      setTrafficStatus('Unable to load edge list from plan yet. Start Live polling to detect edges.');
      return;
    }
    const edges = deriveEdgeNames(data.nodes || [], data.links || []);
    syncTrafficSelectors(edges);
    if (edges.length === 0) {
      setTrafficStatus('No edge nodes detected for this lab.');
    } else {
      setTrafficStatus('Select source/target edge nodes.');
    }
  }

  function syncTrafficSelectors(edges, forceClear) {
    const source = $('liveTrafficSource');
    const target = $('liveTrafficTarget');
    if (!source || !target) return;
    const unique = Array.from(new Set((edges || []).filter(Boolean))).sort();
    if (unique.length === 0 && !forceClear) return;
    liveTraffic.edges = unique;
    const sourceCurrent = source.value;
    const targetCurrent = target.value;
    const options = ['<option value="">Select...</option>']
      .concat(liveTraffic.edges.map(n => `<option value="${n}">${n}</option>`))
      .join('');
    source.innerHTML = options;
    target.innerHTML = options;
    if (liveTraffic.edges.includes(sourceCurrent)) source.value = sourceCurrent;
    if (liveTraffic.edges.includes(targetCurrent)) target.value = targetCurrent;
  }

  function deriveEdgeNames(nodes, links) {
    const out = new Set();
    (nodes || []).forEach(n => {
      if (!n || !n.name) return;
      const role = (n.role || '').toLowerCase();
      if (role === 'edge' || isEdgeName(n.name)) out.add(n.name);
    });
    (links || []).forEach(l => {
      if (l && isEdgeName(l.a)) out.add(l.a);
      if (l && isEdgeName(l.b)) out.add(l.b);
    });
    return Array.from(out).sort();
  }

  function isEdgeName(name) {
    if (!name) return false;
    return /^edge\d+$/i.test(String(name));
  }

  function selectTrafficNode(nodeName) {
    const source = $('liveTrafficSource');
    const target = $('liveTrafficTarget');
    if (!source || !target || !nodeName) return;
    if (!source.value || source.value === nodeName) {
      source.value = nodeName;
      return;
    }
    target.value = nodeName;
  }

  async function runTraffic() {
    const labName = $('liveLabSelect') ? $('liveLabSelect').value : '';
    const source = $('liveTrafficSource') ? $('liveTrafficSource').value : '';
    const target = $('liveTrafficTarget') ? $('liveTrafficTarget').value : '';
    const count = parseInt($('liveTrafficCount').value, 10);
    if (!labName || !source || !target) {
      setTrafficStatus('Select lab, source, and target first.');
      return;
    }
    if (source === target) {
      setTrafficStatus('Source and target must be different.');
      return;
    }
    setTrafficStatus('Running ping traffic...');
    const payload = {
      labName,
      sudo: $('liveUseSudo').value === 'true',
      source,
      target,
      count: Number.isFinite(count) ? count : 5
    };
    const res = await fetch('/topology/traffic', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      setTrafficStatus(`Traffic failed: ${data.error || 'unknown error'}`);
      $('liveDetails').textContent = JSON.stringify(data, null, 2);
      setTrafficOverlay(source, target, false);
      if (lastLiveData) renderLiveGraph(lastLiveData);
      return;
    }
    const success = pingReceivedCount(data.output || '') > 0;
    setTrafficOverlay(source, target, success);
    setTrafficStatus(`Traffic complete: ${source} -> ${target} (${data.targetIp})`);
    $('liveDetails').textContent = JSON.stringify(data, null, 2);
    if (lastLiveData) renderLiveGraph(lastLiveData);
    pollLive();
  }

  function setTrafficOverlay(source, target, success) {
    trafficOverlay = {
      source,
      target,
      success,
      expiresAt: Date.now() + 12000
    };
    if (trafficOverlayTimer) clearTimeout(trafficOverlayTimer);
    trafficOverlayTimer = setTimeout(() => {
      trafficOverlay = null;
      if (lastLiveData) renderLiveGraph(lastLiveData);
    }, 12000);
  }

  function pingReceivedCount(output) {
    const patterns = [
      /(\d+)\s+packets?\s+received/i,
      /(\d+)\s+received/i
    ];
    for (const re of patterns) {
      const m = re.exec(output || '');
      if (m && m[1]) return parseInt(m[1], 10) || 0;
    }
    return 0;
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
    const edgeNames = deriveEdgeNames(nodes, links);
    syncTrafficSelectors(edgeNames);
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

    renderTrafficOverlay(svg, layout, links);

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
      group.style.cursor = 'pointer';
      group.addEventListener('click', () => {
        if (isServer) {
          selectTrafficNode(name);
          setTrafficStatus(`Selected ${name}. Pick source and target, then click Generate Traffic.`);
        }
        startLiveTerminalSession(name);
      });
      svg.appendChild(group);
    });
  }

  function renderTrafficOverlay(svg, layout, links) {
    if (!trafficOverlay) return;
    if (Date.now() > trafficOverlay.expiresAt) {
      trafficOverlay = null;
      return;
    }
    const path = shortestPath(links, trafficOverlay.source, trafficOverlay.target);
    if (path.length < 2) return;
    const cls = trafficOverlay.success ? 'live-traffic-path live-traffic-success' : 'live-traffic-path live-traffic-fail';
    for (let i = 0; i < path.length - 1; i++) {
      const a = layout[path[i]];
      const b = layout[path[i + 1]];
      if (!a || !b) continue;
      const line = document.createElementNS(svgNS, 'line');
      line.setAttribute('x1', a.x);
      line.setAttribute('y1', a.y);
      line.setAttribute('x2', b.x);
      line.setAttribute('y2', b.y);
      line.setAttribute('class', cls);
      svg.appendChild(line);
    }
  }

  function shortestPath(links, source, target) {
    if (!source || !target || source === target) return [];
    const adj = new Map();
    (links || []).forEach(l => {
      if (!l || !l.a || !l.b) return;
      if (!adj.has(l.a)) adj.set(l.a, new Set());
      if (!adj.has(l.b)) adj.set(l.b, new Set());
      adj.get(l.a).add(l.b);
      adj.get(l.b).add(l.a);
    });
    if (!adj.has(source) || !adj.has(target)) return [];
    const q = [source];
    const prev = new Map();
    prev.set(source, '');
    while (q.length) {
      const n = q.shift();
      if (n === target) break;
      const next = adj.get(n) || new Set();
      next.forEach(v => {
        if (prev.has(v)) return;
        prev.set(v, n);
        q.push(v);
      });
    }
    if (!prev.has(target)) return [];
    const rev = [];
    let cur = target;
    while (cur) {
      rev.push(cur);
      cur = prev.get(cur) || '';
    }
    return rev.reverse();
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
    syncTrafficSelectors([], true);
    $('liveStartBtn').addEventListener('click', startPolling);
    $('liveStopBtn').addEventListener('click', stopPolling);
    $('liveLabSelect').addEventListener('change', () => {
      const labName = $('liveLabSelect').value;
      closeLiveTerminalSession();
      const panel = $('liveConsolePanel');
      if (panel) panel.hidden = true;
      syncTrafficSelectors([], true);
      loadTrafficEdgesFromPlan(labName);
      pollLive();
    });
    $('liveTrafficRunBtn').addEventListener('click', runTraffic);
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
    $('liveConsoleReconnectBtn').addEventListener('click', () => {
      if (!liveTerminal.node) {
        setStatus('Click a node in the graph to open terminal', 'status-idle');
        return;
      }
      startLiveTerminalSession(liveTerminal.node);
    });
    window.addEventListener('resize', () => {
      if (!liveTerminal.fitAddon || !liveTerminal.xterm) return;
      liveTerminal.fitAddon.fit();
      if (liveTerminal.socket && liveTerminal.socket.readyState === WebSocket.OPEN) {
        liveTerminal.socket.send(JSON.stringify({ type: 'resize', cols: liveTerminal.xterm.cols, rows: liveTerminal.xterm.rows }));
      }
    });
    window.addEventListener('beforeunload', closeLiveTerminalSession);
  });
})();
