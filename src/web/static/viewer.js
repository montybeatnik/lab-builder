(function () {
  if (window.__labViewerInit) return;
  window.__labViewerInit = true;

  function $(id) { return document.getElementById(id); }
  const svgNS = 'http://www.w3.org/2000/svg';

  async function loadLabs() {
    const select = $('viewerLabSelect');
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

  async function loadPlan() {
    const name = $('viewerLabSelect').value;
    if (!name) return;
    const status = $('viewerStatus');
    status.textContent = 'Loading...';
    status.className = 'status status-pending';

    const res = await fetch(`/labplan?name=${encodeURIComponent(name)}`);
    const data = await res.json().catch(() => ({ ok: false }));
    if (!data.ok) {
      status.textContent = 'Load failed';
      status.className = 'status status-fail';
      return;
    }
    status.textContent = 'Loaded';
    status.className = 'status status-pass';
    renderGraph(data);
    renderProtocols(data);
  }

  function renderGraph(data) {
    const svg = $('viewerGraph');
    if (!svg) return;
    svg.innerHTML = '';
    svg.classList.add('future-mode');
    const wrap = svg.closest('.graph-wrap');
    if (wrap) wrap.classList.add('future-wrap');
    const nodes = data.nodes || [];
    const links = data.links || [];
    const nodeNames = nodes.map(n => n.name);
    const edgeNames = collectEdgeNames(links);
    edgeNames.forEach(n => {
      if (!nodeNames.includes(n)) nodeNames.push(n);
    });

    const layout = layoutNodes(nodeNames, nodes);
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
    nodeNames.forEach(name => {
      const pos = layout[name];
      if (!pos) return;
      drawStyledNode(svg, name, pos);
    });
  }

  function drawStyledNode(svg, name, pos) {
    const group = document.createElementNS(svgNS, 'g');
    const isServer = pos.role === 'edge';
    group.setAttribute('class', `node ${pos.role} ${isServer ? 'node-server' : 'node-switch'}`);
    if (isServer) {
      const rect = document.createElementNS(svgNS, 'rect');
      rect.setAttribute('x', pos.x - 26);
      rect.setAttribute('y', pos.y - 13);
      rect.setAttribute('width', 52);
      rect.setAttribute('height', 26);
      rect.setAttribute('rx', 4);
      rect.setAttribute('class', 'server-chassis');
      group.appendChild(rect);
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
      const rect = document.createElementNS(svgNS, 'rect');
      rect.setAttribute('x', pos.x - 30);
      rect.setAttribute('y', pos.y - 11);
      rect.setAttribute('width', 60);
      rect.setAttribute('height', 22);
      rect.setAttribute('rx', 6);
      rect.setAttribute('class', 'switch-chassis');
      group.appendChild(rect);
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
  }

  function collectEdgeNames(links) {
    const set = new Set();
    links.forEach(link => {
      if (link.a && link.a.startsWith('edge')) set.add(link.a);
      if (link.b && link.b.startsWith('edge')) set.add(link.b);
    });
    return Array.from(set);
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

    spine.forEach((n, i) => layout[n] = { x: spineXs[i], y: 120, role: 'spine' });
    leaf.forEach((n, i) => layout[n] = { x: leafXs[i], y: 380, role: 'leaf' });

    const edgeXs = spreadX(edge.length, width, 140);
    edge.forEach((n, i) => layout[n] = { x: edgeXs[i], y: 470, role: 'edge' });

    if (spine.length === 0 && leaf.length === 0) {
      const center = { x: width / 2, y: height / 2 };
      const radius = 160;
      rest.forEach((n, i) => {
        const angle = (Math.PI * 2 * i) / Math.max(rest.length, 1);
        layout[n] = {
          x: center.x + radius * Math.cos(angle),
          y: center.y + radius * Math.sin(angle),
          role: 'mesh'
        };
      });
      return layout;
    }

    const restXs = spreadX(rest.length, width, 160);
    rest.forEach((n, i) => {
      layout[n] = { x: restXs[i], y: 250, role: 'mesh' };
    });
    return layout;
  }

  function spreadX(count, width, margin) {
    if (count <= 1) return [width / 2];
    const usable = width - margin * 2;
    return Array.from({ length: count }, (_, i) => margin + (usable * i) / (count - 1));
  }

  function renderProtocols(data) {
    const wrap = $('viewerProtocols');
    if (!wrap) return;
    const protocols = data.protocols || { global: [], roles: {} };
    const lines = [];
    if (protocols.global && protocols.global.length) {
      lines.push(`Global: ${protocols.global.join(', ')}`);
    }
    Object.keys(protocols.roles || {}).forEach(role => {
      const list = protocols.roles[role] || [];
      if (list.length) lines.push(`${role}: ${list.join(', ')}`);
    });
    wrap.innerHTML = lines.length ? lines.map(l => `<div>${l}</div>`).join('') : '<div class="muted">No protocol data</div>';
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (!$('viewerLabSelect')) return;
    $('viewerLabSelect').addEventListener('change', loadPlan);
    $('viewerRefresh').addEventListener('click', loadPlan);
    loadLabs();
  });
})();
