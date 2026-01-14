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
      select.innerHTML = '<option value="">No labs found</option>';
      return;
    }
    if (!data.labs || data.labs.length === 0) {
      select.innerHTML = '<option value="">No labs found</option>';
      return;
    }
    select.innerHTML = '<option value="">Select a lab...</option>';
    data.labs.forEach(lab => {
      const opt = document.createElement('option');
      opt.value = lab.name;
      opt.setAttribute('data-path', lab.path);
      opt.textContent = `${lab.name} (${lab.path})`;
      select.appendChild(opt);
    });
  }

  function onSelectLab() {
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
    $('labSelect').addEventListener('change', onSelectLab);
    $('inspectBtn').addEventListener('click', runInspect);
    $('healthBtn').addEventListener('click', runHealth);
    loadLabs();

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
  });
})();
