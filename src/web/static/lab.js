(function () {
  if (window.__labTestsInit) return;
  window.__labTestsInit = true;

  function $(id) { return document.getElementById(id); }

  function updateLabFile(path) {
    if (!$('labFile')) return;
    if (path) {
      $('labFile').value = path;
    }
  }

  function populateLabSelect(select, labs) {
    if (!select) return;
    if (!labs || labs.length === 0) {
      select.innerHTML = '<option value="">No labs found</option>';
      return;
    }
    select.innerHTML = '<option value="">Select a lab...</option>';
    labs.forEach(lab => {
      const opt = document.createElement('option');
      opt.value = lab.name;
      opt.setAttribute('data-path', lab.path);
      opt.textContent = `${lab.name} (${lab.path})`;
      select.appendChild(opt);
    });
  }

  async function loadLabNodes() {
    const select = $('nodeConfigSelect');
    if (!select) return;
    const lab = $('labFile').value.trim();
    if (!lab) {
      select.innerHTML = '<option value="">Enter a lab file first</option>';
      return;
    }
    select.innerHTML = '<option value="">Loading...</option>';
    const res = await fetch('/lab/nodes', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ lab })
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok || !data.nodes || data.nodes.length === 0) {
      select.innerHTML = `<option value="">${data.error || 'No nodes found'}</option>`;
      return;
    }
    select.innerHTML = data.nodes.map(node => `<option value="${node}">${node}</option>`).join('');
  }

  function buildBasePayload() {
    return {
      lab: $('labFile').value.trim(),
      timeoutSec: parseInt($('labTimeout').value, 10) || 20,
      sudo: $('labSudo').value === 'true'
    };
  }

  async function loadLabs() {
    const select = $('labSelect');
    if (!select) return;
    const res = await fetch('/labs');
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      populateLabSelect(select, null);
      return;
    }
    populateLabSelect(select, data.labs || []);
  }

  function syncSelectedLab() {
    const select = $('labSelect');
    if (!select) return;
    const name = select.value;
    if (!name) return;
    const path = select.options[select.selectedIndex].getAttribute('data-path');
    if (path) {
      updateLabFile(path);
    } else {
      updateLabFile(`${name}/lab.clab.yml`);
    }
    loadLabNodes();
  }

  async function loadNodeConfig() {
    const status = $('labStatus');
    const node = $('nodeConfigSelect').value;
    const configOut = $('nodeConfigOut');
    const daemonsOut = $('nodeDaemonsOut');
    const startupOut = $('nodeStartupOut');
    configOut.hidden = true;
    daemonsOut.hidden = true;
    startupOut.hidden = true;
    configOut.textContent = '';
    daemonsOut.textContent = '';
    startupOut.textContent = '';
    if (!node) {
      configOut.hidden = false;
      configOut.textContent = 'Select a node first';
      return;
    }

    status.textContent = `Loading ${node} config...`;
    status.className = 'status status-pending';
    const res = await fetch('/lab/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        lab: $('labFile').value.trim(),
        nodeName: node
      })
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      status.textContent = 'Config load failed';
      status.className = 'status status-fail';
      configOut.hidden = false;
      configOut.textContent = data.error || 'config load failed';
      sessionStorage.setItem('lab_node_config', configOut.textContent);
      sessionStorage.setItem('lab_node_daemons', '');
      sessionStorage.setItem('lab_node_startup', '');
      return;
    }

    status.textContent = `${data.nodeName} config loaded`;
    status.className = 'status status-pass';
    configOut.hidden = false;
    configOut.textContent = data.config || '';
    if (data.daemons) {
      daemonsOut.hidden = false;
      daemonsOut.textContent = data.daemons;
    }
    if (data.startup) {
      startupOut.hidden = false;
      startupOut.textContent = data.startup;
    }
    sessionStorage.setItem('lab_node_config', configOut.textContent);
    sessionStorage.setItem('lab_node_daemons', data.daemons || '');
    sessionStorage.setItem('lab_node_startup', data.startup || '');
  }

  async function runInspect() {
    const status = $('labStatus');
    status.textContent = 'Inspecting...';
    status.className = 'status status-pending';
    const payload = buildBasePayload();
    const res = await fetch('/inspect', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    const out = $('inspectOut');
    if (!data.ok) {
      status.textContent = 'Inspect failed';
      status.className = 'status status-fail';
      out.hidden = false;
      out.textContent = data.error || 'inspect failed';
      sessionStorage.setItem('lab_inspect', out.textContent);
      sessionStorage.setItem('lab_inspect_ok', 'false');
      return;
    }
    status.textContent = 'Inspect complete';
    status.className = 'status status-pass';
    out.hidden = false;
    out.textContent = JSON.stringify(data, null, 2);
    sessionStorage.setItem('lab_inspect', out.textContent);
    sessionStorage.setItem('lab_inspect_ok', 'true');
  }

  async function runHealth() {
    const status = $('labStatus');
    status.textContent = 'Running health...';
    status.className = 'status status-pending';
    const base = buildBasePayload();
    const payload = {
      lab: base.lab,
      timeoutSec: base.timeoutSec,
      sudo: base.sudo,
      user: $('healthUser').value,
      pass: $('healthPass').value
    };
    const res = await fetch('/health', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    const out = $('healthOut');
    if (!data.ok) {
      status.textContent = 'Health failed';
      status.className = 'status status-fail';
      out.hidden = false;
      out.textContent = data.error || 'health failed';
      sessionStorage.setItem('lab_health', out.textContent);
      sessionStorage.setItem('lab_health_ok', 'false');
      return;
    }
    status.textContent = 'Health complete';
    status.className = 'status status-pass';
    out.hidden = false;
    out.textContent = JSON.stringify(data, null, 2);
    sessionStorage.setItem('lab_health', out.textContent);
    sessionStorage.setItem('lab_health_ok', 'true');
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (!$('inspectBtn')) return;
    $('labSelect').addEventListener('change', syncSelectedLab);
    $('labFile').addEventListener('change', loadLabNodes);
    $('inspectBtn').addEventListener('click', runInspect);
    $('healthBtn').addEventListener('click', runHealth);
    $('nodeConfigBtn').addEventListener('click', loadNodeConfig);
    loadLabs();
    loadLabNodes();

    const savedInspect = sessionStorage.getItem('lab_inspect');
    if (savedInspect) {
      const out = $('inspectOut');
      out.hidden = false;
      out.textContent = savedInspect;
    }
    const savedHealth = sessionStorage.getItem('lab_health');
    if (savedHealth) {
      const out = $('healthOut');
      out.hidden = false;
      out.textContent = savedHealth;
    }
    const savedNodeConfig = sessionStorage.getItem('lab_node_config');
    if (savedNodeConfig) {
      const out = $('nodeConfigOut');
      out.hidden = false;
      out.textContent = savedNodeConfig;
    }
    const savedNodeDaemons = sessionStorage.getItem('lab_node_daemons');
    if (savedNodeDaemons) {
      const out = $('nodeDaemonsOut');
      out.hidden = false;
      out.textContent = savedNodeDaemons;
    }
    const savedNodeStartup = sessionStorage.getItem('lab_node_startup');
    if (savedNodeStartup) {
      const out = $('nodeStartupOut');
      out.hidden = false;
      out.textContent = savedNodeStartup;
    }
  });
})();
