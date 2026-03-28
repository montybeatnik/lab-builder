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
    const input = $('walkthroughSelected');
    if (!input) return;
    input.value = state.selectedID ? `${name} (${status})` : '';
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

      if (!autoSelected && !state.selectedID && item.status === 'ready') {
        autoSelected = true;
        setSelected(item.id, item.name, item.status);
        state.steps = stepsForWalkthrough(item.id);
        state.stepIndex = 0;
        renderStepper();
        runPreflight(item.id);
      }
    });
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
    const detailHeader = $('walkthroughStepHeader');
    const detailFacts = $('walkthroughStepFacts');
    const detailCommands = $('walkthroughStepCommands');
    const detailValidation = $('walkthroughStepValidation');
    if (!list || !detail || !detailHeader || !detailFacts || !detailCommands || !detailValidation) return;
    if (!state.steps.length) {
      list.innerHTML = '<li class="muted">No steps loaded yet.</li>';
      detailHeader.textContent = '';
      detailFacts.textContent = '';
      detailCommands.innerHTML = '';
      detailValidation.textContent = '';
      return;
    }
    if (state.stepIndex < 0) state.stepIndex = 0;
    if (state.stepIndex >= state.steps.length) state.stepIndex = state.steps.length - 1;
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
