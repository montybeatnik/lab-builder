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
    deployedLabs: [],
    terminalSocket: null,
    xterm: null,
    fitAddon: null,
    resizingPanes: false,
    guideWindow: null,
    guideChannel: null,
    guidePoppedOut: false
  };
  const walkthroughLabByID = {
    'evpn-vxlan-stretched-l2-foundation': 'walkthrough-evpn-vxlan-l2',
    'evpn-vxlan-multihoming': 'walkthrough-evpn-multihoming',
    'evpn-vxlan-routing': 'walkthrough-evpn-vxlan-l3',
    'srv6-foundation': 'walkthrough-srv6-foundation'
  };
  const walkthroughIDByLab = Object.fromEntries(
    Object.entries(walkthroughLabByID).map(([id, lab]) => [lab, id])
  );
  const storageKeys = {
    selectedID: 'walkthrough:selectedID',
    activeLab: 'walkthrough:activeLab'
  };

  function launchBtn() {
    return $('walkthroughLaunchBtn');
  }

  function setLaunchBusy(isBusy, label) {
    const btn = launchBtn();
    if (!btn) return;
    if (!btn.dataset.defaultLabel) {
      btn.dataset.defaultLabel = btn.textContent || 'Deploy Selected Walkthrough';
    }
    btn.disabled = Boolean(isBusy);
    btn.textContent = isBusy ? (label || 'Deploying...') : btn.dataset.defaultLabel;
  }

  function setStatus(text, cls) {
    const el = $('walkthroughStatus');
    if (!el) return;
    el.textContent = text;
    el.className = `status ${cls || 'status-idle'}`;
  }

  function setSelected(id, name, status) {
    state.selectedID = id || '';
    try {
      if (state.selectedID) localStorage.setItem(storageKeys.selectedID, state.selectedID);
      else localStorage.removeItem(storageKeys.selectedID);
    } catch {}
    const input = $('walkthroughSelected');
    if (!input) return;
    input.value = state.selectedID ? `${name} (${status})` : '';
  }

  function setActiveLab(labName) {
    state.activeLab = (labName || '').trim();
    try {
      if (state.activeLab) localStorage.setItem(storageKeys.activeLab, state.activeLab);
      else localStorage.removeItem(storageKeys.activeLab);
    } catch {}
  }

  function showCaptureModal(text) {
    const modal = $('walkthroughCaptureModal');
    const out = $('walkthroughCaptureOutput');
    if (!modal || !out) return;
    out.textContent = text || '';
    modal.hidden = false;
  }

  function hideCaptureModal() {
    const modal = $('walkthroughCaptureModal');
    if (!modal) return;
    modal.hidden = true;
  }

  function updateWorkareaLayout() {
    const workarea = $('walkthroughWorkarea');
    const consolePanel = $('walkthroughConsolePanel');
    if (!workarea || !consolePanel) return;
    if (consolePanel.hidden) {
      workarea.classList.add('no-console');
      return;
    }
    workarea.classList.remove('no-console');
    if (!workarea.style.gridTemplateRows) {
      workarea.style.gridTemplateRows = 'minmax(360px, 62vh) 10px minmax(220px, 38vh)';
    }
  }

  function fitTerminal() {
    if (!state.fitAddon || !state.xterm) return;
    state.fitAddon.fit();
    if (state.terminalSocket && state.terminalSocket.readyState === WebSocket.OPEN) {
      state.terminalSocket.send(JSON.stringify({ type: 'resize', cols: state.xterm.cols, rows: state.xterm.rows }));
    }
  }

  function setGuideDockMode(poppedOut) {
    state.guidePoppedOut = Boolean(poppedOut);
    const runner = $('walkthroughRunner');
    const guidePanel = $('walkthroughGuidePanel');
    const popBtn = $('walkthroughPopoutGuideBtn');
    document.body.classList.toggle('walkthrough-guide-popout-mode', state.guidePoppedOut);
    if (runner) runner.classList.toggle('guide-popped-out', state.guidePoppedOut);
    if (guidePanel) guidePanel.hidden = state.guidePoppedOut;
    if (popBtn) popBtn.textContent = state.guidePoppedOut ? 'Focus Guide Window' : 'Open Guide';
    updateWorkareaLayout();
    fitTerminal();
  }

  function initPaneResizer() {
    const resizer = $('walkthroughPaneResizer');
    const workarea = $('walkthroughWorkarea');
    const consolePanel = $('walkthroughConsolePanel');
    if (!resizer || !workarea || !consolePanel) return;

    const onMove = (ev) => {
      if (!state.resizingPanes || consolePanel.hidden) return;
      const rect = workarea.getBoundingClientRect();
      const handleH = resizer.getBoundingClientRect().height || 10;
      const minTop = 260;
      const minBottom = 180;
      let top = ev.clientY - rect.top;
      top = Math.max(minTop, top);
      top = Math.min(rect.height - minBottom - handleH, top);
      const bottom = Math.max(minBottom, rect.height - top - handleH);
      workarea.style.gridTemplateRows = `${Math.round(top)}px ${Math.round(handleH)}px ${Math.round(bottom)}px`;
      fitTerminal();
    };

    const stopResize = () => {
      if (!state.resizingPanes) return;
      state.resizingPanes = false;
      document.body.style.userSelect = '';
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', stopResize);
    };

    resizer.addEventListener('pointerdown', (ev) => {
      if (consolePanel.hidden) return;
      state.resizingPanes = true;
      document.body.style.userSelect = 'none';
      window.addEventListener('pointermove', onMove);
      window.addEventListener('pointerup', stopResize);
      ev.preventDefault();
    });
  }

  function safeCloneGuideSteps() {
    return (state.steps || []).map((step) => ({
      title: step.title || '',
      goal: step.goal || '',
      commands: Array.isArray(step.commands) ? step.commands : [],
      validate: step.validate || ''
    }));
  }

  function guidePayload() {
    return {
      steps: safeCloneGuideSteps(),
      stepIndex: state.stepIndex,
      selectedID: state.selectedID || '',
      activeLab: state.activeLab || ''
    };
  }

  function ensureGuideChannel() {
    if (!window.BroadcastChannel) return null;
    if (state.guideChannel) return state.guideChannel;
    const channel = new window.BroadcastChannel('walkthrough-guide');
    channel.onmessage = (ev) => {
      const msg = ev && ev.data ? ev.data : {};
      if (msg.type === 'setStepIndex' && Number.isFinite(msg.stepIndex)) {
        state.stepIndex = msg.stepIndex;
        renderStepper();
      }
      if (msg.type === 'closed') {
        state.guideWindow = null;
        setGuideDockMode(false);
      }
      if (msg.type === 'requestState') {
        channel.postMessage({ type: 'state', payload: guidePayload() });
      }
    };
    state.guideChannel = channel;
    return channel;
  }

  function syncGuideWindow() {
    const channel = ensureGuideChannel();
    if (!channel) return;
    channel.postMessage({ type: 'state', payload: guidePayload() });
  }

  function reconcileGuideWindowState() {
    if (!state.guidePoppedOut) return;
    if (state.guideWindow && !state.guideWindow.closed) return;
    state.guideWindow = null;
    setGuideDockMode(false);
  }

  function openGuideWindow() {
    if (state.guideWindow && !state.guideWindow.closed) {
      setGuideDockMode(true);
      state.guideWindow.focus();
      syncGuideWindow();
      return;
    }
    const popup = window.open('', 'walkthrough-guide', 'popup=yes,width=620,height=900,resizable=yes,scrollbars=yes');
    if (!popup) {
      setStatus('Popup blocked by browser', 'status-fail');
      return;
    }
    const doc = popup.document;
    doc.open();
    doc.write(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Walkthrough Guide</title>
  <style>
    :root{color-scheme:dark}
    html,body{height:100%}
    body{margin:0;padding:14px;background:#020617;color:#e2e8f0;font:14px/1.45 ui-sans-serif,system-ui,-apple-system,Segoe UI,sans-serif;box-sizing:border-box}
    *,*:before,*:after{box-sizing:inherit}
    .layout{height:100%;display:grid;grid-template-rows:auto auto minmax(120px,1fr) auto minmax(220px,2fr);gap:10px;min-height:0}
    h1{font-size:20px;margin:0}
    .row{display:flex;align-items:center;justify-content:space-between;gap:10px}
    .muted{color:#94a3b8;font-size:12px}
    .steps{margin:0;padding-left:18px;overflow:auto;min-height:0}
    .steps li{cursor:pointer;margin:2px 0}
    .steps li.active{color:#22d3ee;font-weight:700}
    .card{border:1px solid #334155;background:#0b1227;border-radius:10px;padding:10px;overflow:auto;min-height:0}
    pre{margin:0;background:#020617;border:1px solid #334155;border-radius:8px;padding:8px;white-space:pre-wrap;word-break:break-word}
    details{margin-top:8px}
    summary{cursor:pointer;color:#93c5fd}
    .actions{display:flex;gap:8px}
    button{border:1px solid #334155;background:#0f172a;color:#e2e8f0;border-radius:8px;padding:7px 10px;cursor:pointer}
  </style>
</head>
<body>
  <div class="layout">
    <div class="row">
      <h1>Step-by-Step Guide</h1>
      <button id="closeBtn" type="button">Close</button>
    </div>
    <div id="meta" class="muted"></div>
    <ol id="steps" class="steps"></ol>
    <div class="actions">
      <button id="prevBtn" type="button">Back</button>
      <button id="nextBtn" type="button">Next</button>
    </div>
    <div class="card">
      <div id="title"></div>
      <div id="facts" class="muted"></div>
      <div id="commands"></div>
      <div id="validate" class="muted"></div>
    </div>
  </div>
  <script>
  (function(){
    const channel = window.BroadcastChannel ? new BroadcastChannel('walkthrough-guide') : null;
    const state = { steps: [], stepIndex: 0, selectedID: '', activeLab: '' };
    function byId(id){ return document.getElementById(id); }
    function clamp() {
      if (!state.steps.length) { state.stepIndex = 0; return; }
      if (state.stepIndex < 0) state.stepIndex = 0;
      if (state.stepIndex > state.steps.length - 1) state.stepIndex = state.steps.length - 1;
    }
    function render() {
      clamp();
      const list = byId('steps');
      const title = byId('title');
      const facts = byId('facts');
      const commands = byId('commands');
      const validate = byId('validate');
      byId('meta').textContent = (state.activeLab ? ('Lab: ' + state.activeLab + '  ') : '') + (state.selectedID ? ('Walkthrough: ' + state.selectedID) : '');
      list.innerHTML = '';
      state.steps.forEach((step, idx) => {
        const li = document.createElement('li');
        li.textContent = step.title || ('Step ' + (idx + 1));
        if (idx === state.stepIndex) li.className = 'active';
        li.addEventListener('click', () => {
          state.stepIndex = idx;
          render();
          if (channel) channel.postMessage({ type: 'setStepIndex', stepIndex: state.stepIndex });
        });
        list.appendChild(li);
      });
      if (!state.steps.length) {
        title.textContent = 'No steps loaded yet.';
        facts.textContent = '';
        commands.innerHTML = '';
        validate.textContent = '';
        return;
      }
      const step = state.steps[state.stepIndex];
      title.textContent = step.title || '';
      facts.textContent = step.goal || '';
      commands.innerHTML = '';
      (Array.isArray(step.commands) ? step.commands : []).forEach((cmd, idx) => {
        const d = document.createElement('details');
        d.open = idx === 0;
        const s = document.createElement('summary');
        const pre = document.createElement('pre');
        if (typeof cmd === 'string') {
          s.textContent = 'Shell';
          pre.textContent = cmd;
        } else {
          s.textContent = (cmd.node || 'node') + (cmd.mode ? (' (' + cmd.mode + ')') : '');
          const lines = Array.isArray(cmd.lines) ? cmd.lines : [];
          pre.textContent = cmd.mode === 'vtysh' ? (['vtysh'].concat(lines).join('\\n')) : lines.join('\\n');
        }
        d.appendChild(s);
        d.appendChild(pre);
        commands.appendChild(d);
      });
      validate.textContent = step.validate ? ('Validation: ' + step.validate) : '';
    }
    byId('prevBtn').addEventListener('click', () => {
      state.stepIndex -= 1;
      render();
      if (channel) channel.postMessage({ type: 'setStepIndex', stepIndex: state.stepIndex });
    });
    byId('nextBtn').addEventListener('click', () => {
      state.stepIndex += 1;
      render();
      if (channel) channel.postMessage({ type: 'setStepIndex', stepIndex: state.stepIndex });
    });
    byId('closeBtn').addEventListener('click', () => window.close());
    window.addEventListener('beforeunload', () => {
      if (channel) channel.postMessage({ type: 'closed' });
    });
    if (channel) {
      channel.onmessage = (ev) => {
        const msg = ev && ev.data ? ev.data : {};
        if (msg.type === 'state' && msg.payload) {
          state.steps = Array.isArray(msg.payload.steps) ? msg.payload.steps : [];
          state.stepIndex = Number.isFinite(msg.payload.stepIndex) ? msg.payload.stepIndex : 0;
          state.selectedID = msg.payload.selectedID || '';
          state.activeLab = msg.payload.activeLab || '';
          render();
        }
      };
      channel.postMessage({ type: 'requestState' });
    }
    render();
  })();
  </script>
</body>
</html>`);
    doc.close();
    state.guideWindow = popup;
    setGuideDockMode(true);
    ensureGuideChannel();
    syncGuideWindow();
  }

  function ensureTerminalVisible() {
    const panel = $('walkthroughConsolePanel');
    if (!panel || panel.hidden) return;
    const rect = panel.getBoundingClientRect();
    const viewportH = window.innerHeight || document.documentElement.clientHeight || 800;
    const desiredTop = Math.floor(viewportH * 0.56);
    // If terminal starts below preferred viewport zone, bring it up while
    // keeping topology/steps visible above it.
    if (rect.top > desiredTop) {
      window.scrollBy({ top: rect.top - desiredTop, behavior: 'smooth' });
      return;
    }
    // If terminal bottom is clipped, nudge just enough to expose it.
    const bottomPadding = 16;
    if (rect.bottom > viewportH - bottomPadding) {
      window.scrollBy({ top: rect.bottom - (viewportH - bottomPadding), behavior: 'smooth' });
    }
  }

  function stepsForWalkthrough(id) {
    if (id === 'evpn-vxlan-stretched-l2-foundation') {
      return [
        {
          title: 'Verify Underlay BGP',
          goal: 'Confirm ipv4 unicast BGP adjacencies are established between spine and leaves.',
          commands: [
            {
              node: 'spine1',
              mode: 'vtysh',
              lines: [
                'show ip bgp summary'
              ]
            },
            {
              node: 'leaf1 / leaf2',
              mode: 'vtysh',
              lines: [
                'show ip bgp summary'
              ]
            }
          ],
          validate: 'All expected neighbors should be Established.'
        },
        {
          title: 'Create Bridge/VXLAN Interface',
          goal: 'Configure a local L2 service construct on each leaf.',
          commands: [
            'ip link add br100 type bridge',
            'ip link add vxlan100 type vxlan id 100 local <leaf-loopback> dstport 4789 nolearning',
            'ip link set vxlan100 master br100',
            'ip link set eth2 master br100',
            'ip link set br100 up && ip link set vxlan100 up && ip link set eth2 up'
          ],
          validate: "Run `ip -d link show vxlan100` on each leaf and confirm the interface is present."
        },
        {
          title: 'Enable EVPN AFI/SAFI',
          goal: 'Configure l2vpn evpn address-family and activate the underlay neighbors.',
          commands: [
            {
              node: 'spine1',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <spine-asn>',
                'address-family l2vpn evpn',
                'neighbor <leaf1-loopback> activate',
                'neighbor <leaf2-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            },
            {
              node: 'leaf1',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <leaf1-asn>',
                'address-family l2vpn evpn',
                'neighbor <spine-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            },
            {
              node: 'leaf2',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <leaf2-asn>',
                'address-family l2vpn evpn',
                'neighbor <spine-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            },
            {
              node: 'verify',
              mode: 'vtysh',
              lines: [
                'show bgp l2vpn evpn summary'
              ]
            }
          ],
          validate: 'EVPN sessions should be Established once full config is applied.'
        },
        {
          title: 'Map Interfaces Into Bridge',
          goal: 'Verify edge-facing and vxlan interfaces are in the same bridge domain.',
          commands: ['bridge link', 'bridge vlan show', 'ip -d link show vxlan100'],
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
    if (id === 'evpn-vxlan-multihoming') {
      return [
        {
          title: 'Verify Topology And Underlay',
          goal: 'Confirm 1 spine / 3 leaves are up with ipv4 unicast BGP Established.',
          commands: [
            {
              node: 'spine1',
              mode: 'vtysh',
              lines: [
                'show ip bgp summary'
              ]
            },
            {
              node: 'leaf1 / leaf2 / leaf3',
              mode: 'vtysh',
              lines: [
                'show ip bgp summary'
              ]
            }
          ],
          validate: 'Spine should have 3 established neighbors; each leaf should have spine1 established.'
        },
        {
          title: 'Confirm Edge1 Dual-Homing',
          goal: 'Validate that edge1 has two uplinks (to leaf1 and leaf2) while edge2 remains single-homed.',
          commands: [
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ip -br link',
                'ip route'
              ]
            },
            {
              node: 'edge2',
              mode: 'shell',
              lines: [
                'ip -br link',
                'ip route'
              ]
            }
          ],
          validate: 'edge1 should show two connected uplink interfaces; edge2 should show one uplink.'
        },
        {
          title: 'Build L2 Service On Leaves',
          goal: 'Create br100/vxlan100 on all leaves and attach host-facing ports.',
          commands: [
            'ip link add br100 type bridge',
            'ip link add vxlan100 type vxlan id 100 local <leaf-loopback> dstport 4789 nolearning',
            'ip link set vxlan100 master br100',
            'ip link set eth2 master br100',
            'ip link set br100 up && ip link set vxlan100 up && ip link set eth2 up'
          ],
          validate: 'Run on leaf1, leaf2, leaf3. Verify `ip -d link show vxlan100` and `bridge link`.'
        },
        {
          title: 'Enable EVPN On Spine/Leaves',
          goal: 'Activate l2vpn evpn neighbors for all 3 leaves.',
          commands: [
            {
              node: 'spine1',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <spine-asn>',
                'address-family l2vpn evpn',
                'neighbor <leaf1-loopback> activate',
                'neighbor <leaf2-loopback> activate',
                'neighbor <leaf3-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            },
            {
              node: 'leaf1 / leaf2 / leaf3',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <leaf-asn>',
                'address-family l2vpn evpn',
                'neighbor <spine-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            }
          ],
          validate: 'Use `show bgp l2vpn evpn summary` and ensure EVPN sessions are Established.'
        },
        {
          title: 'Validate Multi-Homed Service',
          goal: 'Prove edge-to-edge reachability and inspect EVPN route state.',
          commands: [
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ping -c 3 <edge2-ip>'
              ]
            },
            {
              node: 'leaf1 / leaf2',
              mode: 'vtysh',
              lines: [
                'show bgp l2vpn evpn route'
              ]
            }
          ],
          validate: 'Ping should succeed; EVPN route output should show learned MAC/IP state across leaves.'
        }
      ];
    }
    if (id === 'evpn-vxlan-routing') {
      return [
        {
          title: 'Verify Underlay Reachability',
          goal: 'Confirm spine/leaf ipv4 unicast BGP is established before enabling EVPN overlay.',
          commands: [
            {
              node: 'spine1',
              mode: 'vtysh',
              lines: [
                'show ip bgp summary'
              ]
            },
            {
              node: 'leaf1 / leaf2',
              mode: 'vtysh',
              lines: [
                'show ip bgp summary'
              ]
            }
          ],
          validate: 'All underlay neighbors should be Established.'
        },
        {
          title: 'Build L2 VNIs On Leaves',
          goal: 'Create bridge and vxlan interfaces for tenant VNIs (example: 100 and 200).',
          commands: [
            {
              node: 'leaf1 / leaf2',
              mode: 'shell',
              lines: [
                'ip link add br100 type bridge',
                'ip link add br200 type bridge',
                'ip link add vxlan100 type vxlan id 100 local <leaf-loopback> dstport 4789 nolearning',
                'ip link add vxlan200 type vxlan id 200 local <leaf-loopback> dstport 4789 nolearning',
                'ip link set vxlan100 master br100',
                'ip link set vxlan200 master br200',
                'ip link set br100 up && ip link set br200 up',
                'ip link set vxlan100 up && ip link set vxlan200 up'
              ]
            }
          ],
          validate: 'Confirm VNI interfaces exist with `ip -d link show vxlan100` and `ip -d link show vxlan200`.'
        },
        {
          title: 'Enable EVPN Address Family',
          goal: 'Activate l2vpn evpn neighbors on spine and leaves and advertise VNIs.',
          commands: [
            {
              node: 'spine1',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <spine-asn>',
                'address-family l2vpn evpn',
                'neighbor <leaf1-loopback> activate',
                'neighbor <leaf2-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            },
            {
              node: 'leaf1 / leaf2',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <leaf-asn>',
                'address-family l2vpn evpn',
                'neighbor <spine-loopback> activate',
                'advertise-all-vni',
                'end',
                'write memory'
              ]
            }
          ],
          validate: 'Use `show bgp l2vpn evpn summary` and verify EVPN sessions are Established.'
        },
        {
          title: 'Configure Anycast Gateway (IRB)',
          goal: 'Create L3 SVIs for each VNI on both leaves using the same anycast gateway IP/MAC.',
          commands: [
            {
              node: 'leaf1 / leaf2',
              mode: 'shell',
              lines: [
                'ip link add vlan100 type dummy',
                'ip link add vlan200 type dummy',
                'ip addr add 172.16.100.1/24 dev vlan100',
                'ip addr add 172.16.200.1/24 dev vlan200',
                'ip link set vlan100 up && ip link set vlan200 up'
              ]
            },
            {
              node: 'leaf1 / leaf2',
              mode: 'vtysh',
              lines: [
                'conf t',
                'router bgp <leaf-asn>',
                'address-family l2vpn evpn',
                'advertise ipv4 unicast',
                'end',
                'write memory'
              ]
            }
          ],
          validate: 'Both leaves should advertise type-5 prefixes for tenant subnets.'
        },
        {
          title: 'Place Endpoints Into Different VNIs',
          goal: 'Attach edge1 to subnet/VNI 100 and edge2 to subnet/VNI 200 with gateway on 172.16.x.1.',
          commands: [
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ip addr flush dev eth1',
                'ip addr add 172.16.100.11/24 dev eth1',
                'ip route add default via 172.16.100.1'
              ]
            },
            {
              node: 'edge2',
              mode: 'shell',
              lines: [
                'ip addr flush dev eth1',
                'ip addr add 172.16.200.22/24 dev eth1',
                'ip route add default via 172.16.200.1'
              ]
            }
          ],
          validate: 'Check `ip route` on each edge and verify the default route points to the anycast gateway.'
        },
        {
          title: 'Validate Inter-VNI Routing',
          goal: 'Confirm routed east-west traffic between VNI 100 and VNI 200 endpoints.',
          commands: [
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ping -c 3 172.16.200.22'
              ]
            },
            {
              node: 'leaf1 / leaf2',
              mode: 'vtysh',
              lines: [
                'show bgp l2vpn evpn route type prefix'
              ]
            }
          ],
          validate: 'Ping should succeed and EVPN type-5 routes should be visible on leaves.'
        }
      ];
    }
    if (id === 'srv6-foundation') {
      return [
        {
          title: 'Why SRv6 In This Design',
          goal: 'Understand SRv6 as an IPv6-native way to steer edge-to-edge traffic through explicit service paths without a separate MPLS data plane.',
          commands: [
            'Design synopsis:',
            '- Use SRv6 to encode service intent in packet headers.',
            '- Avoid per-flow state in transit nodes by pushing segment lists at the edge.',
            '- Keep operations IPv6-native with standard Linux tooling from edge hosts to fabric nodes.'
          ],
          validate: 'You can explain where SRv6 policy is applied (ingress), where SIDs are consumed (transit/egress), and why this simplifies service steering.'
        },
        {
          title: 'Baseline Node/Link Checks',
          goal: 'Confirm all fabric and edge nodes are up and discover interface mapping.',
          commands: [
            {
              node: 'node1 / node2 / node3 / edge1 / edge2',
              mode: 'shell',
              lines: [
                'ip -br link',
                'ip -6 addr show'
              ]
            }
          ],
          validate: 'Identify links: node1-eth1<->node2-eth1, node1-eth2<->node3-eth1, node2-eth2<->node3-eth2, edge1-eth1<->node1-eth3, edge2-eth1<->node3-eth3.'
        },
        {
          title: 'Build IPv6 Transport + Edge Access',
          goal: 'Create IPv6 underlay connectivity, add SRv6 locator loopbacks, and provision IPv6 access links for edge hosts.',
          commands: [
            {
              node: 'node1',
              mode: 'shell',
              lines: [
                'sysctl -w net.ipv6.conf.all.forwarding=1',
                'sysctl -w net.ipv6.conf.default.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.all.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.eth1.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.eth2.seg6_enabled=1',
                'ip -6 addr add 2001:db8:12::1/64 dev eth1',
                'ip -6 addr add 2001:db8:13::1/64 dev eth2',
                'ip -6 addr add 2001:db8:1e1::1/64 dev eth3',
                'ip -6 addr add 2001:db8:100:1::1/128 dev lo'
              ]
            },
            {
              node: 'node2',
              mode: 'shell',
              lines: [
                'sysctl -w net.ipv6.conf.all.forwarding=1',
                'sysctl -w net.ipv6.conf.default.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.all.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.eth1.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.eth2.seg6_enabled=1',
                'ip -6 addr add 2001:db8:12::2/64 dev eth1',
                'ip -6 addr add 2001:db8:23::2/64 dev eth2',
                'ip -6 addr add 2001:db8:100:2::1/128 dev lo',
                'ip -6 addr del 2001:db8:100:2::100/128 dev lo 2>/dev/null || true'
              ]
            },
            {
              node: 'node3',
              mode: 'shell',
              lines: [
                'sysctl -w net.ipv6.conf.all.forwarding=1',
                'sysctl -w net.ipv6.conf.default.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.all.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.eth1.seg6_enabled=1',
                'sysctl -w net.ipv6.conf.eth2.seg6_enabled=1',
                'ip -6 addr add 2001:db8:13::3/64 dev eth1',
                'ip -6 addr add 2001:db8:23::3/64 dev eth2',
                'ip -6 addr add 2001:db8:3e2::1/64 dev eth3',
                'ip -6 addr add 2001:db8:100:3::1/128 dev lo',
                'ip -6 addr del 2001:db8:100:3::100/128 dev lo 2>/dev/null || true'
              ]
            },
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ip -6 addr add 2001:db8:1e1::2/64 dev eth1',
                'ip -6 route replace default via 2001:db8:1e1::1 dev eth1'
              ]
            },
            {
              node: 'edge2',
              mode: 'shell',
              lines: [
                'ip -6 addr add 2001:db8:3e2::2/64 dev eth1',
                'ip -6 route replace default via 2001:db8:3e2::1 dev eth1'
              ]
            }
          ],
          validate: 'Verify each fabric node has transport IPv6 addresses plus locator /128, and each edge has IPv6 + default route.'
        },
        {
          title: 'Add Static IPv6 Reachability',
          goal: 'Install static routes for locator and edge-access prefixes before SR policy steering.',
          commands: [
            {
              node: 'node1',
              mode: 'shell',
              lines: [
                'ip -6 route replace 2001:db8:100:2::1/128 via 2001:db8:12::2 dev eth1',
                'ip -6 route replace 2001:db8:100:2::100/128 via 2001:db8:12::2 dev eth1',
                'ip -6 route replace 2001:db8:100:3::1/128 via 2001:db8:13::3 dev eth2',
                'ip -6 route replace 2001:db8:100:3::100/128 via 2001:db8:13::3 dev eth2',
                'ip -6 route replace 2001:db8:3e2::/64 via 2001:db8:13::3 dev eth2',
                'ping -6 -c 2 2001:db8:12::2'
              ]
            },
            {
              node: 'node2',
              mode: 'shell',
              lines: [
                'ip -6 route replace 2001:db8:100:1::1/128 via 2001:db8:12::1 dev eth1',
                'ip -6 route replace 2001:db8:100:3::1/128 via 2001:db8:23::3 dev eth2',
                'ip -6 route replace 2001:db8:100:3::100/128 via 2001:db8:23::3 dev eth2',
                'ip -6 route replace 2001:db8:3e2::/64 via 2001:db8:23::3 dev eth2',
                'ip -6 route replace local 2001:db8:100:2::100/128 encap seg6local action End.X nh6 2001:db8:23::3 dev eth2'
              ]
            },
            {
              node: 'node3',
              mode: 'shell',
              lines: [
                'ip -6 route replace 2001:db8:100:1::1/128 via 2001:db8:13::1 dev eth1',
                'ip -6 route replace 2001:db8:100:2::1/128 via 2001:db8:23::2 dev eth2',
                'ip -6 route replace 2001:db8:100:2::100/128 via 2001:db8:23::2 dev eth2',
                'ip -6 route replace 2001:db8:1e1::/64 via 2001:db8:13::1 dev eth1',
                'ip -6 route replace local 2001:db8:100:3::100/128 encap seg6local action End.DX6 nh6 2001:db8:3e2::2 dev eth3'
              ]
            },
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ping -6 -c 3 2001:db8:3e2::2'
              ]
            }
          ],
          validate: 'Baseline edge-to-edge ping should work before SRv6 steering. Keep SID addresses (`::100`) as route-only locals, not loopback interface addresses.'
        },
        {
          title: 'Program SRv6 Policy At Ingress',
          goal: 'Steer edge1->edge2 traffic at node1 through node2 and then into node3.',
          commands: [
            {
              node: 'node1',
              mode: 'shell',
              lines: [
                'ip -6 route replace 2001:db8:3e2::/64 encap seg6 mode encap segs 2001:db8:100:2::100,2001:db8:100:3::100 via 2001:db8:12::2 dev eth1'
              ]
            }
          ],
          validate: 'Check `ip -6 -d route show 2001:db8:3e2::/64` and confirm `encap seg6` segment list is present.'
        },
        {
          title: 'Operational Verification',
          goal: 'Confirm SRv6 route programming and forwarding state on ingress/transit/endpoint nodes.',
          commands: [
            {
              node: 'node1',
              mode: 'shell',
              lines: [
                'ip -6 route show 2001:db8:3e2::/64',
                'ip -6 -d route show'
              ]
            },
            {
              node: 'node2 / node3',
              mode: 'shell',
              lines: [
                'ip -6 route show',
                'ip -6 neigh show',
                'sysctl net.ipv6.conf.all.forwarding',
                'sysctl net.ipv6.conf.all.seg6_enabled',
                'sysctl net.ipv6.conf.eth1.seg6_enabled',
                'sysctl net.ipv6.conf.eth2.seg6_enabled',
                'ip -6 route show table local | grep seg6local || true'
              ]
            },
            {
              node: 'edge1 / edge2',
              mode: 'shell',
              lines: [
                'ip -6 route show'
              ]
            }
          ],
          validate: 'Ingress shows SRv6 encapsulation route; transit/egress show healthy IPv6 adjacency/forwarding state.'
        },
        {
          title: 'Data Plane Test + Capture',
          goal: 'Generate traffic and validate forwarding path with packet capture evidence.',
          commands: [
            {
              node: 'edge1',
              mode: 'shell',
              lines: [
                'ping -6 -c 5 2001:db8:3e2::2'
              ]
            },
            {
              node: 'node2',
              mode: 'shell',
              lines: [
                'tcpdump -nn -i any -c 30 ip6'
              ]
            }
          ],
          validate: 'Ping succeeds and capture output confirms IPv6 packets traversing the expected transit node.'
        }
      ];
    }
    return [
      {
        title: 'Inspect Topology And Baseline',
        goal: 'Confirm all nodes are running and underlay adjacencies are healthy before applying walkthrough-specific config.',
        commands: [
          {
            node: 'spine / leaves',
            mode: 'vtysh',
            lines: [
              'show ip bgp summary'
            ]
          },
          {
            node: 'edge nodes',
            mode: 'shell',
            lines: [
              'ip -br a',
              'ip route'
            ]
          }
        ],
        validate: 'All expected sessions and interfaces should be present before proceeding.'
      },
      {
        title: 'Apply Scenario Config',
        goal: 'Apply the intended walkthrough config to fabric and edge nodes in small increments.',
        commands: [
          'Apply config in the node terminal based on walkthrough objective.',
          'Commit/Save config where needed.'
        ],
        validate: 'No command errors and expected control-plane state appears.'
      },
      {
        title: 'Run End-To-End Validation',
        goal: 'Verify data plane and control plane from both endpoint and leaf perspectives.',
        commands: [
          {
            node: 'edge nodes',
            mode: 'shell',
            lines: [
              'ping -c 3 <remote-endpoint-ip>'
            ]
          },
          {
            node: 'leaf nodes',
            mode: 'vtysh',
            lines: [
              'show bgp l2vpn evpn summary'
            ]
          }
        ],
        validate: 'Traffic succeeds and EVPN/BGP state is stable.'
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
    let autoSelected = false;
    let preferredID = '';
    try {
      preferredID = (localStorage.getItem(storageKeys.selectedID) || '').trim();
    } catch {}
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
        setActiveLab('');
        state.steps = stepsForWalkthrough(item.id);
        state.stepIndex = 0;
        renderStepper();
        setStatus('Selected walkthrough', 'status-pass');
        runPreflight(item.id);
      });
      body.appendChild(row);

      if (!autoSelected && item.status === 'ready' && (preferredID === item.id || (!preferredID && !state.selectedID))) {
        autoSelected = true;
        setSelected(item.id, item.name, item.status);
        state.steps = stepsForWalkthrough(item.id);
        state.stepIndex = 0;
        renderStepper();
        runPreflight(item.id);
      }
    });
    if (!autoSelected) {
      // Fallback for cases where stored selection is unavailable/not ready.
      const firstReady = data.items.find(i => i.status === 'ready');
      if (firstReady) {
        setSelected(firstReady.id, firstReady.name, firstReady.status);
        state.steps = stepsForWalkthrough(firstReady.id);
        state.stepIndex = 0;
        renderStepper();
        runPreflight(firstReady.id);
      }
    }
  }

  async function runPreflight(id) {
    const out = $('walkthroughOutput');
    if (!id) return;
    setLaunchBusy(true, 'Checking...');
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
    setLaunchBusy(false);
    if (!data.ok) {
      setStatus('Preflight failed', 'status-fail');
      return;
    }
    if (data.labName) {
      setActiveLab(data.labName);
    }
    state.deployedLabs = Array.isArray(data.deployedLabs) ? data.deployedLabs.slice() : [];
    if (Array.isArray(data.deployedLabs) && data.deployedLabs.length > 0) {
      const walkthroughLabs = data.deployedLabs.filter(name => /^walkthrough-/i.test(name));
      if (walkthroughLabs.length > 0) {
        const resumeLab = walkthroughLabs[0];
        setActiveLab(resumeLab);
        const resumeID = walkthroughIDByLab[resumeLab] || '';
        if (resumeID) {
          state.selectedID = resumeID;
          try { localStorage.setItem(storageKeys.selectedID, resumeID); } catch {}
          state.steps = stepsForWalkthrough(resumeID);
          state.stepIndex = 0;
          renderStepper();
        }
        await loadPlanAndRender();
        $('walkthroughRunner').hidden = false;
        setStatus(`Resumed deployed walkthrough lab: ${resumeLab}`, 'status-pass');
        return;
      }
      setStatus('Existing deployed lab detected', 'status-pending');
      return;
    }
    setStatus('Environment clear for launch', 'status-pass');
  }

  async function launch(forceReplace) {
    const out = $('walkthroughOutput');
    if (!state.selectedID) {
      setStatus('Select a walkthrough first', 'status-idle');
      out.hidden = false;
      out.textContent = 'No walkthrough selected. Choose one from the catalog first.';
      return;
    }
    setLaunchBusy(true, 'Deploying...');
    setStatus('Launching walkthrough lab...', 'status-pending');
    out.hidden = false;
    out.textContent = 'Launching walkthrough lab...';
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
    setLaunchBusy(false);

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
    const walkthroughID = data.walkthroughId || state.selectedID;
    state.steps = stepsForWalkthrough(walkthroughID);
    state.stepIndex = 0;
    setActiveLab(data.labName || '');
    await loadPlanAndRender();
    $('walkthroughRunner').hidden = false;
  }

  async function loadPlanAndRender() {
    if (!state.activeLab) {
      setStatus('No active walkthrough lab selected', 'status-idle');
      return;
    }
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
    const detailHeader = $('walkthroughStepHeader');
    const detailFacts = $('walkthroughStepFacts');
    const detailCommands = $('walkthroughStepCommands');
    const detailValidation = $('walkthroughStepValidation');

    if (!state.steps.length) {
      if (list) list.innerHTML = '<li class="muted">No steps loaded yet.</li>';
      if (detailHeader) detailHeader.textContent = '';
      if (detailFacts) detailFacts.textContent = '';
      if (detailCommands) detailCommands.innerHTML = '';
      if (detailValidation) detailValidation.textContent = '';
      syncGuideWindow();
      return;
    }
    if (state.stepIndex < 0) state.stepIndex = 0;
    if (state.stepIndex >= state.steps.length) state.stepIndex = state.steps.length - 1;
    if (list) {
      list.innerHTML = '';
      state.steps.forEach((s, idx) => {
        const li = document.createElement('li');
        li.className = `walkthrough-step${idx === state.stepIndex ? ' active' : ''}`;
        li.textContent = s.title;
        li.addEventListener('click', () => {
          state.stepIndex = idx;
          renderStepper();
        });
        list.appendChild(li);
      });
    }
    const step = state.steps[state.stepIndex];
    const nodeFacts = (state.plan?.nodes || []).map(n => `${n.name}(asn=${n.asn},loopback=${n.loopback || '-'})`);
    const leafLoopbacks = (state.plan?.nodes || [])
      .filter(n => n.role === 'leaf' && n.loopback)
      .map(n => `${n.name}=${n.loopback}`);
    const spineLoopbacks = (state.plan?.nodes || [])
      .filter(n => n.role === 'spine' && n.loopback)
      .map(n => `${n.name}=${n.loopback}`);
    const factLines = [`Goal: ${step.goal}`];
    if (leafLoopbacks.length) {
      factLines.push('• Leaf loopbacks:');
      leafLoopbacks.forEach(v => factLines.push(`- ${v}`));
    }
    if (spineLoopbacks.length) {
      factLines.push('• Spine loopbacks:');
      spineLoopbacks.forEach(v => factLines.push(`- ${v}`));
    }
    if (nodeFacts.length) {
      factLines.push(`• Node facts: ${nodeFacts.join(', ')}`);
    }
    if (!detail || !detailHeader || !detailFacts || !detailCommands || !detailValidation) {
      syncGuideWindow();
      return;
    }

    detailHeader.textContent = `Step ${state.stepIndex + 1}: ${step.title}`;
    detailFacts.textContent = factLines.join('\n');
    detailCommands.innerHTML = '';
    const commandList = step.commands || [];
    const shellCommands = commandList.filter(cmd => typeof cmd === 'string');
    const commandGroups = [];
    if (shellCommands.length > 0) {
      commandGroups.push({
        title: 'Shell',
        body: shellCommands.join('\n')
      });
    }
    commandList.forEach((cmd) => {
      if (typeof cmd === 'string') return;
      const title = `${cmd.node || 'node'}${cmd.mode ? ` (${cmd.mode})` : ''}`;
      const lines = Array.isArray(cmd.lines) ? cmd.lines : [];
      let text = '';
      if (cmd.mode === 'vtysh') {
        text = ['vtysh', ...lines].join('\n');
      } else {
        text = lines.join('\n');
      }
      commandGroups.push({ title, body: text });
    });
    commandGroups.forEach((group, idx) => {
      const wrapper = document.createElement('details');
      wrapper.className = 'walkthrough-command-block walkthrough-command-accordion';
      wrapper.open = idx === 0;
      const summary = document.createElement('summary');
      summary.className = 'walkthrough-command-title';
      summary.textContent = group.title;
      const body = document.createElement('pre');
      body.textContent = group.body;
      wrapper.appendChild(summary);
      wrapper.appendChild(body);
      wrapper.addEventListener('toggle', () => {
        if (!wrapper.open) return;
        detailCommands.querySelectorAll('.walkthrough-command-accordion').forEach(other => {
          if (other !== wrapper) other.open = false;
        });
      });
      detailCommands.appendChild(wrapper);
    });

    detailValidation.textContent = `Validation: ${step.validate}`;
    syncGuideWindow();
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
    updateWorkareaLayout();
    $('walkthroughConsoleNode').textContent = `Selected node: ${name}`;
    ensureTerminalVisible();
    await startTerminalSession();
    // Re-run after terminal init so final height changes are accounted for.
    setTimeout(ensureTerminalVisible, 50);
  }

  function appendTerminal(text) {
    if (!text) return;
    if (state.xterm) {
      state.xterm.write(text);
      state.xterm.scrollToBottom();
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
      ensureTerminalVisible();
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

  async function runCapture() {
    if (!state.activeLab || !state.activeNode) {
      setStatus('Select a node first for capture', 'status-idle');
      return;
    }
    setStatus(`Capturing traffic on ${state.activeNode}...`, 'status-pending');
    const payload = {
      labName: state.activeLab,
      nodeName: state.activeNode,
      command: "if ! command -v tcpdump >/dev/null 2>&1; then if command -v apt-get >/dev/null 2>&1; then apt-get update >/dev/null 2>&1 && DEBIAN_FRONTEND=noninteractive apt-get install -y tcpdump >/dev/null 2>&1; elif command -v apk >/dev/null 2>&1; then apk add --no-cache tcpdump >/dev/null 2>&1; fi; fi; if command -v tcpdump >/dev/null 2>&1; then timeout 12 tcpdump -nn -i any -c 40 '(ip or ip6)' 2>&1; else echo 'tcpdump not available on this node'; fi",
      sudo: $('walkthroughUseSudo').value === 'true',
      timeoutSec: 30
    };
    const res = await fetch('/walkthroughs/terminal', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json().catch(() => ({ ok: false, error: 'bad response' }));
    if (!data.ok) {
      setStatus('Capture failed', 'status-fail');
      showCaptureModal(`Capture failed:\n${data.error || 'unknown error'}\n\n${data.output || ''}`);
      return;
    }
    setStatus(`Capture complete on ${state.activeNode}`, 'status-pass');
    showCaptureModal(data.output || '(no capture output)');
  }

  document.addEventListener('DOMContentLoaded', () => {
    if (!$('walkthroughRows')) return;
    hideCaptureModal();
    setGuideDockMode(false);
    updateWorkareaLayout();
    initPaneResizer();
    ensureGuideChannel();
    loadCatalog();
    $('walkthroughLaunchBtn').addEventListener('click', () => launch(false));
    $('walkthroughUseSudo').addEventListener('change', () => {
      if (state.selectedID) runPreflight(state.selectedID);
    });
    $('walkthroughRefreshTopoBtn').addEventListener('click', async () => {
      if (state.activeLab) {
        await loadPlanAndRender();
        return;
      }
      if (state.selectedID) {
        await runPreflight(state.selectedID);
        return;
      }
      setStatus('Select a walkthrough first', 'status-idle');
    });
    const prevBtn = $('walkthroughPrevStepBtn');
    if (prevBtn) {
      prevBtn.addEventListener('click', () => {
        state.stepIndex -= 1;
        renderStepper();
      });
    }
    const nextBtn = $('walkthroughNextStepBtn');
    if (nextBtn) {
      nextBtn.addEventListener('click', () => {
        state.stepIndex += 1;
        renderStepper();
      });
    }
    $('walkthroughConsoleReconnectBtn').addEventListener('click', startTerminalSession);
    $('walkthroughCaptureBtn').addEventListener('click', runCapture);
    const popoutBtn = $('walkthroughPopoutGuideBtn');
    if (popoutBtn) popoutBtn.addEventListener('click', openGuideWindow);
    $('walkthroughCaptureCloseBtn').addEventListener('click', hideCaptureModal);
    $('walkthroughCaptureModal').addEventListener('click', (ev) => {
      if (ev.target && ev.target.id === 'walkthroughCaptureModal') hideCaptureModal();
    });
    window.addEventListener('resize', () => {
      fitTerminal();
    });
    window.addEventListener('focus', reconcileGuideWindowState);
    document.addEventListener('visibilitychange', reconcileGuideWindowState);
    window.addEventListener('beforeunload', closeTerminalSession);
  });
})();
