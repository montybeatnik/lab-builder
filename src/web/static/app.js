(function () {
  if (window.__labBuilderInit) return;
  window.__labBuilderInit = true;

  function $(id) { return document.getElementById(id); }

  const topoSelectors = ['leaf-spine', 'full-mesh', 'hub-spoke', 'custom'];
  const svgNS = 'http://www.w3.org/2000/svg';

  function showTopoFields() {
    const topo = $('topology').value;
    document.querySelectorAll('[data-topo]').forEach(el => {
      const allowed = el.getAttribute('data-topo').split(' ');
      el.style.display = allowed.includes(topo) ? '' : 'none';
    });
  }

  function currentNodeNames() {
    const topo = $('topology').value;
    if (topo === 'leaf-spine') {
      const spines = numberVal('spines', 2);
      const leaves = numberVal('leaves', 4);
      return [
        ...Array.from({ length: spines }, (_, i) => `spine${i + 1}`),
        ...Array.from({ length: leaves }, (_, i) => `leaf${i + 1}`)
      ];
    }
    if (topo === 'full-mesh') {
      const nodes = numberVal('meshNodes', 6);
      return Array.from({ length: nodes }, (_, i) => `node${i + 1}`);
    }
    if (topo === 'hub-spoke') {
      const hubs = numberVal('hubs', 1);
      const spokes = numberVal('spokes', 6);
      return [
        ...Array.from({ length: hubs }, (_, i) => `hub${i + 1}`),
        ...Array.from({ length: spokes }, (_, i) => `spoke${i + 1}`)
      ];
    }
    const nodes = numberVal('customNodes', 6);
    return Array.from({ length: nodes }, (_, i) => `node${i + 1}`);
  }

  function numberVal(id, fallback) {
    const val = parseInt($(id).value, 10);
    if (Number.isFinite(val)) return val;
    return fallback;
  }

  function buildModelFromInputs() {
    const topo = $('topology').value;
    const nodes = currentNodeNames();
    const links = [];

    if (topo === 'leaf-spine') {
      const spines = numberVal('spines', 2);
      const leaves = numberVal('leaves', 4);
      for (let i = 1; i <= spines; i++) {
        for (let j = 1; j <= leaves; j++) {
          links.push({ a: `spine${i}`, b: `leaf${j}` });
        }
      }
    } else if (topo === 'full-mesh') {
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          links.push({ a: nodes[i], b: nodes[j] });
        }
      }
    } else if (topo === 'hub-spoke') {
      const hubs = numberVal('hubs', 1);
      const spokes = numberVal('spokes', 6);
      for (let i = 1; i <= hubs; i++) {
        for (let j = 1; j <= spokes; j++) {
          links.push({ a: `hub${i}`, b: `spoke${j}` });
        }
      }
    } else if (topo === 'custom') {
      document.querySelectorAll('.link-row').forEach(row => {
        const a = row.querySelector('[data-end="a"]').value;
        const b = row.querySelector('[data-end="b"]').value;
        if (a && b && a !== b) {
          links.push({ a, b });
        }
      });
    }

    return { topology: topo, nodes, links };
  }

  function renderGraph() {
    const svg = $('graph');
    const model = buildModelFromInputs();
    svg.innerHTML = '';

    const viewWidth = 1000;
    const viewHeight = 520;
    const layout = layoutNodes(model, viewWidth, viewHeight);

    model.links.forEach(link => {
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

    model.nodes.forEach(name => {
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

  function layoutNodes(model, width, height) {
    const topo = $('topology').value;
    const layout = {};
    if (topo === 'leaf-spine') {
      const spines = numberVal('spines', 2);
      const leaves = numberVal('leaves', 4);
      const spineXs = spreadX(spines, width, 120);
      const leafXs = spreadX(leaves, width, 120);
      for (let i = 0; i < spines; i++) {
        layout[`spine${i + 1}`] = { x: spineXs[i], y: 120, role: 'spine' };
      }
      for (let i = 0; i < leaves; i++) {
        layout[`leaf${i + 1}`] = { x: leafXs[i], y: 380, role: 'leaf' };
      }
    } else if (topo === 'hub-spoke') {
      const hubs = numberVal('hubs', 1);
      const spokes = numberVal('spokes', 6);
      const center = { x: width / 2, y: height / 2 - 10 };
      const hubXs = spreadX(hubs, width, 180);
      for (let i = 0; i < hubs; i++) {
        layout[`hub${i + 1}`] = { x: hubXs[i], y: center.y, role: 'spine' };
      }
      const radius = Math.min(width, height) / 2.6;
      for (let i = 0; i < spokes; i++) {
        const angle = (Math.PI * 2 * i) / Math.max(spokes, 1);
        layout[`spoke${i + 1}`] = {
          x: center.x + radius * Math.cos(angle),
          y: center.y + radius * Math.sin(angle),
          role: 'leaf'
        };
      }
    } else {
      const nodes = currentNodeNames();
      const radius = Math.min(width, height) / 2.4;
      const center = { x: width / 2, y: height / 2 - 10 };
      nodes.forEach((name, i) => {
        const angle = (Math.PI * 2 * i) / Math.max(nodes.length, 1);
        layout[name] = {
          x: center.x + radius * Math.cos(angle),
          y: center.y + radius * Math.sin(angle),
          role: 'mesh'
        };
      });
    }
    return layout;
  }

  function spreadX(count, width, margin) {
    if (count <= 1) return [width / 2];
    const usable = width - margin * 2;
    return Array.from({ length: count }, (_, i) => margin + (usable * i) / (count - 1));
  }

  function syncCustomLinks() {
    const rows = $('linkRows');
    const nodes = currentNodeNames();
    rows.querySelectorAll('select').forEach(select => {
      const current = select.value;
      select.innerHTML = nodes.map(n => `<option value="${n}">${n}</option>`).join('');
      if (nodes.includes(current)) select.value = current;
    });
  }

  function addLinkRow(a, b) {
    const row = document.createElement('div');
    row.className = 'link-row';
    row.innerHTML = `
      <select data-end="a"></select>
      <span class="link-arrow">&lt;-&gt;</span>
      <select data-end="b"></select>
      <button type="button" class="ghost remove-link">Remove</button>
    `;
    $('linkRows').appendChild(row);
    syncCustomLinks();
    if (a) row.querySelector('[data-end="a"]').value = a;
    if (b) row.querySelector('[data-end="b"]').value = b;
    row.querySelector('.remove-link').addEventListener('click', () => {
      row.remove();
      renderGraph();
    });
    row.querySelectorAll('select').forEach(select => {
      select.addEventListener('change', renderGraph);
    });
  }

  function collectTraffic() {
    const profiles = [
      { id: 'Voice', on: $('trafficVoice').checked, level: numberVal('levelVoice', 3) },
      { id: 'Video', on: $('trafficVideo').checked, level: numberVal('levelVideo', 4) },
      { id: 'Email', on: $('trafficEmail').checked, level: numberVal('levelEmail', 2) },
      { id: 'Web', on: $('trafficWeb').checked, level: numberVal('levelWeb', 3) }
    ];
    return profiles.filter(p => p.on).map(p => ({ profile: p.id.toLowerCase(), level: p.level }));
  }

  async function validateTopology() {
    const payload = {
      topology: $('topology').value,
      nodeType: $('nodeType').value,
      nodeCount: numberVal('meshNodes', 0),
      leafCount: numberVal('leaves', 0),
      spineCount: numberVal('spines', 0),
      hubCount: numberVal('hubs', 0),
      spokeCount: numberVal('spokes', 0),
      edgeNodes: numberVal('edgeNodes', 0),
      infraCidr: $('infraCidr').value.trim(),
      edgeCidr: $('edgeCidr').value.trim(),
      customLinks: collectCustomLinks(),
      traffic: collectTraffic(),
      protocols: collectProtocols(),
      edgeLinks: collectEdgeLinks(),
      monitoring: collectMonitoring()
    };
    const status = $('buildStatus');
    status.textContent = 'Validating...';
    status.className = 'status status-pending';

    const res = await fetch('/topology/validate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, errors: ['bad response'] }));
    renderChecks(data);
    renderPlan(data);
    if (data.ok) {
      status.textContent = 'Topology ready';
      status.className = 'status status-pass';
    } else {
      status.textContent = 'Needs attention';
      status.className = 'status status-fail';
    }
  }

  async function buildLab() {
    const payload = {
      topology: $('topology').value,
      nodeType: $('nodeType').value,
      nodeCount: numberVal('meshNodes', 0),
      leafCount: numberVal('leaves', 0),
      spineCount: numberVal('spines', 0),
      hubCount: numberVal('hubs', 0),
      spokeCount: numberVal('spokes', 0),
      edgeNodes: numberVal('edgeNodes', 0),
      infraCidr: $('infraCidr').value.trim(),
      edgeCidr: $('edgeCidr').value.trim(),
      customLinks: collectCustomLinks(),
      traffic: collectTraffic(),
      protocols: collectProtocols(),
      edgeLinks: collectEdgeLinks(),
      monitoring: collectMonitoring(),
      labName: $('labName').value.trim(),
      force: $('forceBuild').value === 'true'
    };

    const result = $('buildResult');
    result.hidden = true;
    result.textContent = '';

    const res = await fetch('/topology/build', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      result.className = 'build-result fail';
      result.textContent = data.error || 'build failed';
      result.hidden = false;
      sessionStorage.setItem('builder_build_result', result.textContent);
      sessionStorage.setItem('builder_build_ok', 'false');
      return;
    }
    const files = (data.files || []).join('\n');
    const warnings = (data.warnings || []).length ? `\nWarnings:\n${data.warnings.join('\n')}` : '';
    result.className = 'build-result pass';
    result.textContent = `Lab generated at ${data.path}\n${files}${warnings}`;
    result.hidden = false;
    sessionStorage.setItem('builder_build_result', result.textContent);
    sessionStorage.setItem('builder_build_ok', 'true');
  }

  async function deployLab() {
    const payload = {
      labName: $('labName').value.trim(),
      sudo: $('deploySudo').value === 'true',
      force: $('forceBuild').value === 'true'
    };
    const result = $('deployResult');
    const status = $('deployStatus');
    result.hidden = true;
    result.textContent = '';
    status.hidden = false;
    status.className = 'status-bar running';

    const res = await fetch('/topology/deploy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      status.className = 'status-bar fail';
      status.querySelector('.status-bar-text').textContent = 'Deploy failed';
      result.className = 'build-result fail';
      result.textContent = (data.error || 'deploy failed') + (data.output ? `\n${data.output}` : '');
      result.hidden = false;
      sessionStorage.setItem('builder_deploy_result', result.textContent);
      sessionStorage.setItem('builder_deploy_ok', 'false');
      return;
    }
    status.className = 'status-bar pass';
    status.querySelector('.status-bar-text').textContent = 'Deploy complete';
    result.className = 'build-result pass';
    result.textContent = `Deploy finished for ${data.path}\n${data.output || ''}`.trim();
    result.hidden = false;
    sessionStorage.setItem('builder_deploy_result', result.textContent);
    sessionStorage.setItem('builder_deploy_ok', 'true');
  }

  function collectCustomLinks() {
    const links = [];
    document.querySelectorAll('.link-row').forEach(row => {
      const a = row.querySelector('[data-end="a"]').value;
      const b = row.querySelector('[data-end="b"]').value;
      if (a && b) links.push({ a, b });
    });
    return links;
  }

  function collectProtocols() {
    const roles = {};
    document.querySelectorAll('.lane-drop').forEach(lane => {
      const role = lane.getAttribute('data-drop');
      const items = Array.from(lane.querySelectorAll('.protocol-pill')).map(p => p.getAttribute('data-proto'));
      roles[role] = items.filter(Boolean);
    });
    return { global: roles.global || [], roles: roles };
  }

  function collectEdgeLinks() {
    const links = [];
    document.querySelectorAll('.edge-attach-row').forEach(row => {
      const edge = row.getAttribute('data-edge');
      const target = row.querySelector('select').value;
      if (edge && target && target !== 'auto') {
        links.push({ edge, target });
      }
    });
    return links;
  }

  function collectMonitoring() {
    return {
      snmp: $('monSnmp') ? $('monSnmp').checked : false,
      gnmi: $('monGnmi') ? $('monGnmi').checked : false
    };
  }

  function renderChecks(data) {
    const wrap = $('checks');
    wrap.innerHTML = '';
    const checks = data.checks || [];
    if (!checks.length) {
      wrap.innerHTML = '<div class="muted">No validation results yet.</div>';
      return;
    }
    checks.forEach(ch => {
      const div = document.createElement('div');
      const cls = ch.result === 'PASS' ? 'pass' : (ch.result === 'WARN' ? 'warn' : 'fail');
      div.className = `check ${cls}`;
      div.innerHTML = `
        <strong>${ch.name}</strong>
        <span class="pill ${cls}">${ch.result}</span>
        <p>${ch.detail || ''}</p>
      `;
      wrap.appendChild(div);
    });
    sessionStorage.setItem('builder_checks', JSON.stringify(checks));
  }

  function renderPlan(data) {
    const pre = $('planJson');
    const payload = {
      model: data.model,
      address: data.address,
      warnings: data.warnings || [],
      errors: data.errors || []
    };
    pre.textContent = JSON.stringify(payload, null, 2);
    sessionStorage.setItem('builder_plan', JSON.stringify(payload));
  }

  function attachListeners() {
    $('topology').addEventListener('change', () => {
      showTopoFields();
      syncCustomLinks();
      renderGraph();
      renderEdgeAttachments();
    });
    ['spines', 'leaves', 'meshNodes', 'hubs', 'spokes', 'customNodes', 'edgeNodes'].forEach(id => {
      const el = $(id);
      if (!el) return;
      el.addEventListener('input', () => {
        syncCustomLinks();
        renderGraph();
        renderEdgeAttachments();
      });
    });
    $('addLinkBtn').addEventListener('click', () => {
      const nodes = currentNodeNames();
      addLinkRow(nodes[0], nodes[1] || nodes[0]);
      renderGraph();
    });
    $('refreshGraph').addEventListener('click', renderGraph);
    $('validateBtn').addEventListener('click', validateTopology);
    $('buildBtn').addEventListener('click', buildLab);
    $('deployBtn').addEventListener('click', deployLab);
  }

  function renderEdgeAttachments() {
    const wrap = $('edgeAttachments');
    if (!wrap) return;
    const edges = numberVal('edgeNodes', 0);
    const nodes = currentNodeNames();
    wrap.innerHTML = '';
    if (edges <= 0) {
      wrap.innerHTML = '<div class="muted">No edge nodes.</div>';
      return;
    }
    for (let i = 1; i <= edges; i++) {
      const row = document.createElement('div');
      row.className = 'edge-attach-row';
      row.setAttribute('data-edge', `edge${i}`);
      row.innerHTML = `
        <div class="edge-label">edge${i}</div>
        <select>
          <option value="auto">auto</option>
          ${nodes.map(n => `<option value="${n}">${n}</option>`).join('')}
        </select>
      `;
      wrap.appendChild(row);
    }
  }

  function setupProtocolDrag() {
    const palette = $('protocolPalette');
    if (!palette) return;
    palette.querySelectorAll('.protocol-chip').forEach(chip => {
      chip.addEventListener('dragstart', e => {
        e.dataTransfer.setData('text/plain', chip.getAttribute('data-proto'));
        e.dataTransfer.effectAllowed = 'copy';
      });
    });

    document.querySelectorAll('.lane-drop').forEach(lane => {
      lane.addEventListener('dragover', e => {
        e.preventDefault();
        lane.classList.add('dragover');
      });
      lane.addEventListener('dragleave', () => lane.classList.remove('dragover'));
      lane.addEventListener('drop', e => {
        e.preventDefault();
        lane.classList.remove('dragover');
        const proto = e.dataTransfer.getData('text/plain');
        if (!proto) return;
        addProtocolPill(lane, proto);
      });
    });
  }

  function addProtocolPill(lane, proto) {
    if (lane.querySelector(`.protocol-pill[data-proto="${proto}"]`)) return;
    const pill = document.createElement('div');
    pill.className = 'protocol-pill';
    pill.setAttribute('data-proto', proto);
    pill.innerHTML = `<span>${proto.toUpperCase()}</span><button type="button" aria-label="remove">x</button>`;
    pill.querySelector('button').addEventListener('click', () => pill.remove());
    lane.appendChild(pill);
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (!$('topology')) return;
    showTopoFields();
    addLinkRow('node1', 'node2');
    attachListeners();
    renderGraph();
    setupProtocolDrag();
    renderEdgeAttachments();

    const savedChecks = sessionStorage.getItem('builder_checks');
    if (savedChecks) {
      try { renderChecks({ checks: JSON.parse(savedChecks) }); } catch {}
    }
    const savedPlan = sessionStorage.getItem('builder_plan');
    if (savedPlan) {
      try { $('planJson').textContent = JSON.stringify(JSON.parse(savedPlan), null, 2); } catch {}
    }
    const savedBuild = sessionStorage.getItem('builder_build_result');
    if (savedBuild) {
      const ok = sessionStorage.getItem('builder_build_ok') === 'true';
      const result = $('buildResult');
      result.className = `build-result ${ok ? 'pass' : 'fail'}`;
      result.textContent = savedBuild;
      result.hidden = false;
    }
    const savedDeploy = sessionStorage.getItem('builder_deploy_result');
    if (savedDeploy) {
      const ok = sessionStorage.getItem('builder_deploy_ok') === 'true';
      const result = $('deployResult');
      result.className = `build-result ${ok ? 'pass' : 'fail'}`;
      result.textContent = savedDeploy;
      result.hidden = false;
    }
  });
})();
