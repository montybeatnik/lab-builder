# Arista CLAB Lab
If you want to experiment with a protocol or technology in the networking space, this lab setup can help quickly build the scaffolding. 

It leverages containerlab, which uses docker under the hood to spin up nodes. You define the nodes and edges with a manifest file declared in YAML. 

- [containlab](https://containerlab.dev)

There is some manual setup if you're using VirtualBox. If you're using multipass, it becomes much easier. 

## Setup High Level Overview
You'll need to do the following:
1. Download the depenencies
3. If using VirtualBox, 
   1. Spin up a VM with a script.
      1. Note, you'll have to "unmount" the drive used for initial boot/os install. 
   2. SCP the EOS image onto the VM. 
      1. The Multipass solution mounts the directory on the host into the VM. 
4. Pull down the repo onto the VM and run the setup script.
5. Have fun with the lab! 

## What we're building
![topo](docs/images/topo.png)

## Downloads
- oracle VirtualBox
- linux [image](https://cdimage.ubuntu.com/releases/24.04/release/ubuntu-24.04.3-live-server-arm64.iso)
- Container EOS image: [cEOSarm-lab-4.34.2.1F.tar.xz](https://www.arista.com/en/support/software-download)

## Setup Steps
1. Download Oracle VirtualBox.
2. Download ubuntu image.
3. Download container EOS image. 
### The Easy Way
1. Run the VM setup script. 
    ```bash
    chmod +x vmsetup.sh
    ./vmsetup.sh
    ```

### Multipass (recommended)
Use the Makefile to stand up the Multipass VM (named `lab-builder`), install Docker + Containerlab, and mount the repo into the VM.
```bash
make vm_setup
```

Build the Go server and install the systemd user service:
```bash
make vm_server_build
make vm_server_install
```

Start the Go UI server inside the VM (systemd user service):
```bash
make vm_server_start
make vm_server_status_service
```

If you need to stop it:
```bash
make vm_server_stop_service
```

If the VM gets into a bad state or multipass hangs, rebuild the VM:
```bash
multipass stop lab-builder
multipass delete --purge lab-builder
make vm_setup
make vm_server_build
make vm_server_install
make vm_server_start
```

If you only need to rebuild the Go server after code changes:
```bash
make vm_rebuild
```

Start the Go UI server inside the VM (foreground, manual run):
```bash
make vm_server
```

Or run it in the background and tail logs:
```bash
make vm_server_bg
make vm_server_logs
```

Expose the UI port and print the VM URL:
```bash
make vm_ui
```

Deploy the lab (handles gNMIc ARM64 pre-pull and Containerlab run dir):
```bash
make vm_deploy
```

Expose monitoring ports and print URLs (Grafana + Prometheus):
```bash
make vm_monitoring
```

### The Harder Way
1. Create new VM with image in VirtualBox 
   1. Open VirtualBox and click on "New".
   2. Name your VM (e.g., "clab-test").
   3. Set the Type to "Linux", subtype to "Ubuntu" and Version to "Ubuntu (64-bit)".
   4. Allocate memory (RAM) to the VM (16 GB recommended).
   5. Allocate cores (CPUs) to the VM (16 recommended).
   6. Create a virtual hard disk (VDI format, dynamically allocated, at least 25 GB).
2. Attach the Ubuntu Server ISO:
   1. Select your newly created VM and click on "Settings".
   2. Navigate to "Storage".
   3. Under "Controller: IDE", click on the empty disk icon.
   4. Click on the disk icon next to "Optical Drive" and choose "Choose a disk file...".
   5. Select the Ubuntu Server ISO you downloaded earlier.
3. Walk through the setup of your machine prompt by prompt. 

* If you want to see screenshots: [image-walkthrough](docs/image_walkthrough.md). 

Once the VM is up. 
1. disconnect from the VPN. 
2. push your ssh key to the machine
   1. 
   ```bash
    ssh-copy-id -i <path-to-pub-key-file>.pub <username>@<vm-IP>
   ```
3. SSH the EOS image on to the VM
   1. Example:
   ```bash
   (ncpcli) christopherhern@Christophers-MacBook-Pro arista-lab % scp ~/Downloads/cEOSarm-lab-4.34.2.1F.tar.xz <username>@<vm-IP>:<homedir>
   This key is not known by any other names.
      cEOSarm-lab-4.34.2.1F.tar.xz                                                                                                                                                                                 100%  550MB  67.9MB/s   00:08
   ```
4. Run the setup script. 
   1. 
   ```bash
   chern@clab-test:~/src/github.com/montybeatnik/arista-lab$ chmod +x setup-clab-on-vritualbox.sh
   chern@clab-test:~/src/github.com/montybeatnik/arista-lab$ ./setup-clab-on-vritualbox.sh
   ```


You should see something simliar to the following:
```bash
╭──────────────────────────────┬─────────────────────┬─────────┬───────────────────╮
│             Name             │      Kind/Image     │  State  │   IPv4/6 Address  │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-gpu1   │ linux               │ running │ 172.20.20.11      │
│                              │ alpine:3.19         │         │ 3fff:172:20:20::b │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-gpu2   │ linux               │ running │ 172.20.20.3       │
│                              │ alpine:3.19         │         │ 3fff:172:20:20::3 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-gpu3   │ linux               │ running │ 172.20.20.2       │
│                              │ alpine:3.19         │         │ 3fff:172:20:20::2 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-gpu4   │ linux               │ running │ 172.20.20.8       │
│                              │ alpine:3.19         │         │ 3fff:172:20:20::8 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-leaf1  │ ceos                │ running │ 172.20.20.7       │
│                              │ ceosimage:4.34.2.1f │         │ 3fff:172:20:20::7 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-leaf2  │ ceos                │ running │ 172.20.20.4       │
│                              │ ceosimage:4.34.2.1f │         │ 3fff:172:20:20::4 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-leaf3  │ ceos                │ running │ 172.20.20.10      │
│                              │ ceosimage:4.34.2.1f │         │ 3fff:172:20:20::a │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-leaf4  │ ceos                │ running │ 172.20.20.5       │
│                              │ ceosimage:4.34.2.1f │         │ 3fff:172:20:20::5 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-spine1 │ ceos                │ running │ 172.20.20.9       │
│                              │ ceosimage:4.34.2.1f │         │ 3fff:172:20:20::9 │
├──────────────────────────────┼─────────────────────┼─────────┼───────────────────┤
│ clab-evpn-rdma-fabric-spine2 │ ceos                │ running │ 172.20.20.6       │
│                              │ ceosimage:4.34.2.1f │         │ 3fff:172:20:20::6 │
╰──────────────────────────────┴─────────────────────┴─────────┴───────────────────╯
```

```bash
# 1) watch tx packet counter
sudo docker exec -it clab-evpn-rdma-fabric-gpu4 sh -lc '
  cat /sys/class/net/eth1/statistics/tx_packets ;
  ping -c3 -W1 10.10.10.101 || true ;
  cat /sys/class/net/eth1/statistics/tx_packets
'

# 2) force a gratuitous ARP + broadcast
sudo docker exec -it clab-evpn-rdma-fabric-gpu4 sh -lc '
  ip neigh flush all ;
  arping -c 3 -I eth1 10.10.10.104 || true
'  # if arping missing, apk add iputils-arping
```

```bash
# deploy lab
sudo containerlab deploy -t lab.clab.yml # may need to add --reconfigure
# inspect lab 
sudo containerlab inspect -t lab.clab.yml
# destroy lab
sudo containerlab destroy -t lab.clab.yml
# view topo 
sudo containerlab graph -t lab.clab.yml
```

## Monitoring
Grafana and Prometheus are exposed on the VM when you deploy the lab with the updated topology.

Quick access (Multipass):
```bash
make vm_monitoring
```

Manual access:
1. Find the VM IP: `multipass info lab-builder`
2. Grafana: `http://<vm-ip>:3000` (default user/pass `admin`/`admin`)
3. Prometheus: `http://<vm-ip>:9090`

### Prometheus: useful queries
Open Prometheus at `http://<vm-ip>:9090` and use the query bar.

Basic health:
```promql
up
```

SNMP scrape health:
```promql
up{job="snmp"}
```

SNMP scrape duration (seconds):
```promql
snmp_scrape_duration_seconds
```

SNMP PDUs returned per scrape:
```promql
snmp_scrape_pdus_returned
```

gNMI scrape health:
```promql
up{job="gnmi"}
```

If you are unsure which metrics exist, open `http://<vm-ip>:9090/targets` and click the gNMI target to view `/metrics`.

### Interface statistics (SNMP)
The default SNMP walk already includes interface counters. Try these PromQL queries:

Inbound/outbound octets (per interface):
```promql
rate(ifHCInOctets[5m])
rate(ifHCOutOctets[5m])
```

Inbound/outbound errors (per interface):
```promql
rate(ifInErrors[5m])
rate(ifOutErrors[5m])
```

Tip: If a metric name differs, open `http://<vm-ip>:9090/targets`, click the SNMP target, and search for `ifHC` or `ifIn` in `/metrics`.

If SNMP scrapes time out, confirm the EOS configs include:
```
snmp-server community public ro
```
Then redeploy the lab so the startup configs are applied.

### BGP neighbors (gNMI)
By default the gNMI config only subscribes to interface counters. To collect BGP neighbor state, add a BGP subscription path in `monitoring/gnmic.yml`, then redeploy:
```yaml
subscriptions:
  interfaces:
    path: /interfaces/interface/state/counters
    stream-mode: sample
    sample-interval: 10s
  bgp_neighbors:
    path: /network-instances/network-instance/protocols/protocol/bgp/neighbors/neighbor/state
    stream-mode: sample
    sample-interval: 10s
```

After redeploy, you can query for neighbor state. Typical fields include:
```promql
gnmi_openconfig_network_instance_protocols_protocol_bgp_neighbors_neighbor_state_session_state
```
If the exact name differs, use Grafana Explore or the Prometheus target `/metrics` to find the exported BGP metric names.

### Containerlab DNS naming
Containerlab node names are exposed in Docker DNS as `clab-<lab-name>-<node-name>`. The default monitoring configs use that full name.
If you change your lab name or node names, update the targets accordingly.

## Clean rebuild (Multipass)
If you want to reset everything and rebuild the VM from scratch:
```bash
multipass stop lab-builder
multipass delete --purge lab-builder
make vm_setup
make vm_server_build
make vm_server_install
make vm_server_start
make vm_deploy
make vm_monitoring
```

### Grafana: quick dashboard setup
Grafana is pre-provisioned with the Prometheus datasource.

1. Open Grafana at `http://<vm-ip>:3000` (default `admin`/`admin`).
2. Go to **Dashboards → New → New dashboard → Add visualization**.
3. Choose the **Prometheus** datasource.
4. Add panels using these example queries:
   - `up{job="snmp"}`
   - `snmp_scrape_duration_seconds`
   - `up{job="gnmi"}`
5. Save the dashboard.

Troubleshooting (ARM64 gNMIc):
If you see `exec format error` for gNMIc and your Containerlab version doesn't support `platform:` in the topology file, pre-pull the ARM64 image before deploy:
```bash
sudo docker pull --platform=linux/arm64 ghcr.io/openconfig/gnmic:latest
```
If the container still fails, remove and re-pull to ensure the correct architecture is cached:
```bash
sudo docker image rm ghcr.io/openconfig/gnmic:latest
sudo docker pull --platform=linux/arm64 ghcr.io/openconfig/gnmic:latest
```

## Creds 
- user: admin
- pass: admin

## Verify 
```text
show bgp summary
show ip route 10.0.0.1
show ip route 10.0.0.2
show bgp evpn summary
show bgp evpn route-type mac-ip
show vxlan vtep
show vxlan address-table
```

### From GPU1 
```bash
sudo docker exec -it clab-evpn-rdma-fabric-gpu1 sh -lc 'ping -c3 10.10.10.104'
```

## Debug 
### Restart docker 
```bash
sudo snap restart docker
```

## MTU ISSUES
```bash
sudo docker exec -it clab-evpn-rdma-fabric-gpu1 sh -lc 'ip link set eth1 mtu 500'
sudo docker exec -it clab-evpn-rdma-fabric-gpu2 sh -lc 'ip link set eth1 mtu 500'
sudo docker exec -it clab-evpn-rdma-fabric-gpu3 sh -lc 'ip link set eth1 mtu 500'
sudo docker exec -it clab-evpn-rdma-fabric-gpu4 sh -lc 'ip link set eth1 mtu 500'
```

## Wireshark Capture
### To run sudo run 
```bash
sudo visudo 
# add this line to the bottom
${USER}   ALL=(ALL) NOPASSWD: ALL
## where is your username; for example:
# chern    ALL=(ALL) NOPASSWD: ALL
```

```bash
ssh {VM_IP} "ip netns exec clab-evpn-rdma-fabric-leaf1 tcpdump -U -nni eth1 -w -" | wireshark -k -i -
# ssh 10.0.0.215 "sudo ip netns exec clab-evpn-rdma-fabric-leaf1 tcpdump -U -nni eth1 -w -" | wireshark -k -i -
ssh 192.168.2.62 "sudo ip netns exec clab-arista-lab-leaf1 tcpdump -U -nni eth1 -w -" | wireshark -k -i - 
```


```
leaf1#show vxlan config-sanity
! Your configuration contains warnings. This does not mean misconfigurations. But you may wish to re-check your configurations.
Category                            Result  Detail
---------------------------------- -------- ----------------------------------
Local VTEP Configuration Check       FAIL
  Flood List                         FAIL   No flood list configured
  Flood List                         FAIL   No remote VTEP in VLAN 10
```

### Unable to start lab try this:
```bash
# inside the VM
mkdir -p ~/.clab-runs
export CLAB_LABDIR_BASE="$HOME/.clab-runs"   # where Containerlab will write the clab-<name>/ dir
sudo -E containerlab destroy -t ~/lab/lab.clab.yml || true
sudo -E containerlab deploy  -t ~/lab/lab.clab.yml --reconfigure
```

## TODO:
- [ ] add multiple topologies
  - [ ] base IP setup between devices
  - [ ] ISIS as IGP 
  - [ ] OSPF as IGP 
  - [ ] Baisc RSVP/MPLS with an IGP
  - [ ] Segment Routing with MPLS
  - [ ] Segment Routing with IPv6
  
