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
    stepIndex: 0
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
        commands: [
          "vtysh -c 'show ip bgp summary'"
        ],
        validate: 'All expected neighbors should be Established.'
      },
      {
        title: 'Create Bridge/VXLAN Interface',
        goal: 'Configure a local L2 service construct on each leaf.',
        commands: [
          "vtysh -c 'conf t' -c 'interface vxlan100' -c 'vxlan id 100' -c 'vxlan local-tunnelip <leaf-loopback>' -c 'exit' -c 'end'"
        ],
        validate: "Run `ip -d link show vxlan100` on each leaf and confirm the interface is present."
      },
      {
        title: 'Enable EVPN AFI/SAFI',
        goal: 'Turn up l2vpn evpn neighbor activation under BGP.',
        commands: [
          "vtysh -c 'conf t' -c 'router bgp <asn>' -c 'address-family l2vpn evpn' -c 'neighbor <peer> activate' -c 'advertise-all-vni' -c 'end'"
        ],
        validate: "Use `vtysh -c 'show bgp l2vpn evpn summary'` and check Established state."
      },
      {
        title: 'Map Edge Interfaces To Bridge',
        goal: 'Attach local edge-facing interface and VXLAN interface into the same bridge domain.',
        commands: [
          'ip link add br100 type bridge',
          'ip link set br100 up',
          'ip link set eth2 master br100',
          'ip link set vxlan100 master br100'
        ],
        validate: "Use `bridge link` and confirm both `eth2` and `vxlan100` are in `br100`."
      },
      {
        title: 'Validate End-To-End Connectivity',
        goal: 'Confirm stretched L2 service by pinging edge-to-edge.',
        commands: [
          'ping -c 3 <edge-peer-ip>'
        ],
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
      if (ok) {
        return launch(true);
      }
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
    if (!state.steps || state.steps.length === 0) {
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
      group.addEventListener('click', () => {
        state.activeNode = name;
        $('walkthroughConsolePanel').hidden = false;
        $('walkthroughConsoleNode').textContent = `Selected node: ${name}`;
        $('walkthroughConsoleOut').textContent = `Node selected: ${name}`;
      });
      svg.appendChild(group);
    });
  }

  async function runNodeCommand() {
    const out = $('walkthroughConsoleOut');
    if (!state.activeLab || !state.activeNode) {
      out.textContent = 'Select a node from the topology first.';
      return;
    }
    const command = $('walkthroughConsoleCmd').value.trim();
    if (!command) {
      out.textContent = 'Enter a command first.';
      return;
    }
    const timeoutSec = parseInt($('walkthroughConsoleTimeout').value, 10);
    out.textContent = `Running on ${state.activeNode}: ${command}\n...`;
    const payload = {
      labName: state.activeLab,
      nodeName: state.activeNode,
      command,
      sudo: $('walkthroughUseSudo').value === 'true',
      timeoutSec: Number.isFinite(timeoutSec) ? timeoutSec : 30
    };
    const res = await fetch('/walkthroughs/terminal', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    out.textContent = JSON.stringify(data, null, 2);
    if (data.ok) {
      setStatus(`Command completed on ${state.activeNode}`, 'status-pass');
    } else {
      setStatus(`Command failed on ${state.activeNode}`, 'status-fail');
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
    $('walkthroughConsoleRunBtn').addEventListener('click', runNodeCommand);
  });
})();

