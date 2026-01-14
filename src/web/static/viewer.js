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
    const rest = names.filter(n => !spine.includes(n) && !leaf.includes(n));

    const width = 1000;
    const height = 520;
    const spineXs = spreadX(spine.length, width, 120);
    const leafXs = spreadX(leaf.length, width, 120);

    spine.forEach((n, i) => layout[n] = { x: spineXs[i], y: 120, role: 'spine' });
    leaf.forEach((n, i) => layout[n] = { x: leafXs[i], y: 380, role: 'leaf' });

    const radius = 120;
    const centerX = width / 2;
    rest.forEach((n, i) => {
      const angle = (Math.PI * 2 * i) / Math.max(rest.length, 1);
      layout[n] = { x: centerX + radius * Math.cos(angle), y: 260 + radius * Math.sin(angle), role: 'mesh' };
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
