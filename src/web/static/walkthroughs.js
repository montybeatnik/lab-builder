(function () {
  if (window.__walkthroughsInit) return;
  window.__walkthroughsInit = true;

  function $(id) { return document.getElementById(id); }
  const svgNS = 'http://www.w3.org/2000/svg';

  const state = {
    selectedID: '',
    activeLab: '',
    activeNode: '',
    plan: null,
    steps: [],
    stepIndex: 0,
    terminalSocket: null,
    xterm: null,
    fitAddon: null
  };

  function setStatus(text, cls) {
    const el = $('walkthroughStatus');
    if (!el) return;
    el.textContent = text;
    el.className = `status ${cls || 'status-idle'}`;
  }

  function setSelected(id, name, status) {
    state.selectedID = id || '';
    const input = $('walkthroughSelected');
    if (!input) return;
    input.value = state.selectedID ? `${name} (${status})` : '';
  }

  function stepsForWalkthrough(id) {
    if (id !== 'evpn-vxlan-stretched-l2-foundation') return [];
    return [
      {
        title: 'Verify Underlay BGP',
        goal: 'Confirm ipv4 unicast BGP adjacencies are established between spine and leaves.',
        commands: ["vtysh -c 'show ip bgp summary'"],
        validate: 'All expected neighbors should be Established.'
      },
      {
        title: 'Create Bridge/VXLAN Interface',
        goal: 'Configure a local L2 service construct on each leaf.',
        commands: ["vtysh -c 'conf t' -c 'interface vxlan100' -c 'vxlan id 100' -c 'vxlan local-tunnelip <leaf-loopback>' -c 'end'"],
        validate: "Run `ip -d link show vxlan100` on each leaf and confirm the interface is present."
      },
      {
        title: 'Enable EVPN AFI/SAFI',
        goal: 'Turn up l2vpn evpn neighbor activation under BGP.',
        commands: ["vtysh -c 'show bgp l2vpn evpn summary'"],
        validate: 'EVPN sessions should be Established once full config is applied.'
      },
      {
        title: 'Map Interfaces Into Bridge',
        goal: 'Attach edge-facing interface and vxlan interface to the bridge domain.',
        commands: ['bridge link', 'ip -d link show'],
        validate: 'Confirm `eth2` and `vxlan100` are in the same bridge domain.'
      },
      {
        title: 'Validate End-To-End Connectivity',
        goal: 'Confirm stretched L2 service by pinging edge-to-edge.',
        commands: ['ping -c 3 <edge-peer-ip>'],
        validate: 'Ping should succeed from edge1 to edge2.'
      }
    ];
  }

  async function loadCatalog() {
    const body = $('walkthroughRows');
    if (!body) return;
    const res = await fetch('/walkthroughs/catalog');
    const data = await res.json().catch(() => ({ ok: false }));
    if (!data.ok || !Array.isArray(data.items) || data.items.length === 0) {
      body.innerHTML = '<tr><td colspan="5" class="muted">No walkthroughs available.</td></tr>';
      return;
    }
    body.innerHTML = '';
    data.items.forEach(item => {
      const row = document.createElement('tr');
      row.innerHTML = `
        <td>${item.name}</td>
        <td>${item.description}</td>
        <td>${item.durationMin} min</td>
        <td><span class="pill ${item.status === 'ready' ? 'pass' : 'planned'}">${item.status.toUpperCase()}</span></td>
        <td><button type="button" class="ghost walkthrough-pick">Select</button></td>
      `;
      const btn = row.querySelector('.walkthrough-pick');
      btn.disabled = item.status !== 'ready';
      btn.addEventListener('click', () => {
        setSelected(item.id, item.name, item.status);
        state.steps = stepsForWalkthrough(item.id);
        state.stepIndex = 0;
        renderStepper();
        setStatus('Selected walkthrough', 'status-pass');
        runPreflight(item.id);
      });
      body.appendChild(row);
    });
  }

  async function runPreflight(id) {
    const out = $('walkthroughOutput');
    if (!id) return;
    setStatus('Checking existing deployed labs...', 'status-pending');
    const payload = {
      walkthroughId: id,
      sudo: $('walkthroughUseSudo').value === 'true'
    };
    const res = await fetch('/walkthroughs/preflight', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    out.hidden = false;
    out.textContent = JSON.stringify(data, null, 2);
    if (!data.ok) {
      setStatus('Preflight failed', 'status-fail');
      return;
    }
    if (Array.isArray(data.deployedLabs) && data.deployedLabs.length > 0) {
      setStatus('Existing deployed lab detected', 'status-pending');
      return;
    }
    setStatus('Environment clear for launch', 'status-pass');
  }

  async function launch(forceReplace) {
    const out = $('walkthroughOutput');
    if (!state.selectedID) {
      setStatus('Select a walkthrough first', 'status-idle');
      return;
    }
    setStatus('Launching walkthrough lab...', 'status-pending');
    const payload = {
      walkthroughId: state.selectedID,
      sudo: $('walkthroughUseSudo').value === 'true',
      forceReplace: Boolean(forceReplace)
    };
    const res = await fetch('/walkthroughs/launch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    out.hidden = false;
    out.textContent = JSON.stringify(data, null, 2);

    if (data.requiresConfirm) {
      const existingList = data.deployedLabs || data.destroyedLabs || [];
      const existing = existingList.join(', ');
      const ok = window.confirm(`A lab is already deployed: ${existing}. Unless you have a super computer, we should tear it down before launching this walkthrough. Continue?`);
      if (ok) return launch(true);
      setStatus('Launch cancelled', 'status-idle');
      return;
    }

    if (!data.ok) {
      setStatus('Launch failed', 'status-fail');
      return;
    }
    setStatus('Walkthrough lab deployed', 'status-pass');
    state.activeLab = data.labName || '';
    await loadPlanAndRender();
    $('walkthroughRunner').hidden = false;
  }

  async function loadPlanAndRender() {
    if (!state.activeLab) return;
    const res = await fetch(`/labplan?name=${encodeURIComponent(state.activeLab)}`);
    const data = await res.json().catch(() => ({ ok: false }));
    if (!data.ok) {
      setStatus('Unable to load walkthrough topology', 'status-fail');
      return;
    }
    state.plan = data;
    renderGraph(data);
    renderStepper();
  }

  function renderStepper() {
    const list = $('walkthroughSteps');
    const detail = $('walkthroughStepDetail');
    if (!list || !detail) return;
    if (!state.steps.length) {
      list.innerHTML = '<li class="muted">No steps loaded yet.</li>';
      detail.textContent = '';
      return;
    }
    if (state.stepIndex < 0) state.stepIndex = 0;
    if (state.stepIndex >= state.steps.length) state.stepIndex = state.steps.length - 1;
    list.innerHTML = '';
    state.steps.forEach((s, idx) => {
      const li = document.createElement('li');
      li.className = `walkthrough-step${idx === state.stepIndex ? ' active' : ''}`;
      li.textContent = `${idx + 1}. ${s.title}`;
      li.addEventListener('click', () => {
        state.stepIndex = idx;
        renderStepper();
      });
      list.appendChild(li);
    });
    const step = state.steps[state.stepIndex];
    detail.textContent = [
      `Step ${state.stepIndex + 1}: ${step.title}`,
      '',
      `Goal: ${step.goal}`,
      '',
      'Suggested commands:',
      ...(step.commands || []).map(c => `  ${c}`),
      '',
      `Validation: ${step.validate}`
    ].join('\n');
  }

  function spreadX(count, width, margin) {
    if (count <= 1) return [width / 2];
    const usable = width - margin * 2;
    return Array.from({ length: count }, (_, i) => margin + (usable * i) / (count - 1));
  }

  function layoutNodes(names, nodes) {
    const layout = {};
    const spine = (nodes || []).filter(n => n.role === 'spine' || n.role === 'hub').map(n => n.name);
    const leaf = (nodes || []).filter(n => n.role === 'leaf' || n.role === 'spoke').map(n => n.name);
    const edge = names.filter(n => /^edge\d+$/i.test(n));
    const rest = names.filter(n => !spine.includes(n) && !leaf.includes(n) && !edge.includes(n));
    const width = 1000;
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

  function renderGraph(data) {
    const svg = $('walkthroughGraph');
    if (!svg) return;
    svg.innerHTML = '';
    const nodes = data.nodes || [];
    const links = data.links || [];
    const names = nodes.map(n => n.name);
    links.forEach(l => {
      if (l.a && !names.includes(l.a)) names.push(l.a);
      if (l.b && !names.includes(l.b)) names.push(l.b);
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
      line.setAttribute('class', 'edge');
      svg.appendChild(line);
    });

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
      const label = document.createElementNS(svgNS, 'text');
      label.setAttribute('x', pos.x);
      label.setAttribute('y', pos.y + 34);
      label.setAttribute('text-anchor', 'middle');
      label.textContent = name;
      group.appendChild(label);
      group.style.cursor = 'pointer';
      group.addEventListener('click', () => selectNode(name));
      svg.appendChild(group);
    });
  }

  async function selectNode(name) {
    state.activeNode = name;
    $('walkthroughConsolePanel').hidden = false;
    $('walkthroughConsoleNode').textContent = `Selected node: ${name}`;
    await startTerminalSession();
  }

  function appendTerminal(text) {
    if (!text) return;
    if (state.xterm) {
      state.xterm.write(text);
      return;
    }
    const screen = $('walkthroughTerminalScreen');
    if (!screen) return;
    screen.textContent += text;
    screen.scrollTop = screen.scrollHeight;
  }

  function initXTerm() {
    const host = $('walkthroughTerminalScreen');
    if (!host) return;
    if (state.xterm) {
      state.xterm.dispose();
      state.xterm = null;
    }
    if (typeof window.Terminal !== 'function') {
      host.textContent = 'Terminal UI failed to load (xterm.js missing).';
      return;
    }
    state.xterm = new window.Terminal({
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
    state.fitAddon = null;
    if (window.FitAddon && typeof window.FitAddon.FitAddon === 'function') {
      state.fitAddon = new window.FitAddon.FitAddon();
      state.xterm.loadAddon(state.fitAddon);
    }
    host.innerHTML = '';
    state.xterm.open(host);
    if (state.fitAddon) state.fitAddon.fit();
    host.addEventListener('click', () => {
      if (state.xterm) state.xterm.focus();
    });
    state.xterm.onData(data => {
      if (!state.terminalSocket || state.terminalSocket.readyState !== WebSocket.OPEN) return;
      state.terminalSocket.send(JSON.stringify({ type: 'input', data }));
    });
    state.xterm.onResize(size => {
      if (!state.terminalSocket || state.terminalSocket.readyState !== WebSocket.OPEN) return;
      state.terminalSocket.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
    });
  }

  async function startTerminalSession() {
    await closeTerminalSession();
    if (!state.activeLab || !state.activeNode) return;
    initXTerm();
    const qs = new URLSearchParams({
      labName: state.activeLab,
      nodeName: state.activeNode,
      sudo: String($('walkthroughUseSudo').value === 'true'),
      timeoutSec: '30'
    });
    const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const wsURL = `${scheme}://${window.location.host}/walkthroughs/terminal/ws?${qs.toString()}`;
    const ws = new WebSocket(wsURL);
    state.terminalSocket = ws;
    ws.addEventListener('open', () => {
      setStatus(`Terminal connected: ${state.activeNode}`, 'status-pass');
      if (state.fitAddon) {
        state.fitAddon.fit();
      }
      if (state.xterm) {
        state.xterm.focus();
        state.xterm.writeln('\x1b[90m[interactive terminal ready]\x1b[0m');
      }
      if (state.xterm) {
        const cols = state.xterm.cols || 80;
        const rows = state.xterm.rows || 24;
        ws.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });
    ws.addEventListener('message', ev => {
      let msg = null;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        appendTerminal(String(ev.data));
        return;
      }
      if (msg.type === 'output') {
        const out = String(msg.data || '');
        appendTerminal(out);
        // Some shells issue DSR (ESC [ 6 n). Reply with a cursor report so the
        // interactive prompt does not stall and users can type immediately.
        if (out.includes('\u001b[6n') || out.includes('[6n')) {
          if (state.terminalSocket && state.terminalSocket.readyState === WebSocket.OPEN) {
            state.terminalSocket.send(JSON.stringify({ type: 'input', data: '\u001b[1;1R' }));
          }
        }
      } else if (msg.type === 'status') {
        appendTerminal(`\n[${msg.data || 'connected'}]\n`);
      } else if (msg.type === 'error') {
        appendTerminal(`\n[error] ${msg.data || 'terminal error'}\n`);
      }
    });
    ws.addEventListener('close', () => {
      if (state.terminalSocket === ws) {
        state.terminalSocket = null;
      }
      appendTerminal('\n[session closed]\n');
    });
    ws.addEventListener('error', () => {
      setStatus('Terminal connection failed', 'status-fail');
    });
  }

  async function closeTerminalSession() {
    if (state.terminalSocket) {
      try {
        state.terminalSocket.send(JSON.stringify({ type: 'close' }));
      } catch {}
      state.terminalSocket.close();
      state.terminalSocket = null;
    }
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (!$('walkthroughRows')) return;
    loadCatalog();
    $('walkthroughLaunchBtn').addEventListener('click', () => launch(false));
    $('walkthroughUseSudo').addEventListener('change', () => {
      if (state.selectedID) runPreflight(state.selectedID);
    });
    $('walkthroughRefreshTopoBtn').addEventListener('click', loadPlanAndRender);
    $('walkthroughPrevStepBtn').addEventListener('click', () => {
      state.stepIndex -= 1;
      renderStepper();
    });
    $('walkthroughNextStepBtn').addEventListener('click', () => {
      state.stepIndex += 1;
      renderStepper();
    });
    $('walkthroughConsoleReconnectBtn').addEventListener('click', startTerminalSession);
    window.addEventListener('resize', () => {
      if (!state.fitAddon || !state.xterm) return;
      state.fitAddon.fit();
      if (state.terminalSocket && state.terminalSocket.readyState === WebSocket.OPEN) {
        state.terminalSocket.send(JSON.stringify({ type: 'resize', cols: state.xterm.cols, rows: state.xterm.rows }));
      }
    });
    window.addEventListener('beforeunload', closeTerminalSession);
  });
})();
