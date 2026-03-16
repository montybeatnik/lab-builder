package configgenerator

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
)

type InterfaceData struct {
	Name string
	IP   string
	Desc string
	L2   bool
	Vlan int
}

type NeighborData struct {
	IP          string
	ASN         int
	Description string
}

type BGPData struct {
	Enabled   bool
	ASN       int
	RouterID  string
	Neighbors []NeighborData
	EVPN      bool
}

type MPLSData struct {
	LDP  bool
	RSVP bool
}

type NodeTemplateData struct {
	Hostname      string
	Loopback      string
	Interfaces    []InterfaceData
	BGP           BGPData
	Protocols     string
	OSPF          string
	ISIS          string
	VXLAN         bool
	VlanID        int
	Vni           int
	EdgeIP        string
	EdgePrefix    int
	EdgeIfName    string
	NodeType      string
	MPLS          MPLSData
	SNMP          bool
	GNMI          bool
	SNMPCommunity string
}

func RenderNodeConfig(tplPath string, node labplanner.NodePlan, links []labplanner.LinkAssigned, nodeMap map[string]labplanner.NodePlan, snmpEnabled, gnmiEnabled bool) (string, error) {
	data := NodeTemplateData{
		Hostname:      node.Name,
		Loopback:      node.Loopback,
		NodeType:      node.NodeType,
		SNMP:          snmpEnabled,
		GNMI:          gnmiEnabled,
		SNMPCommunity: "public",
	}

	for _, link := range links {
		var (
			peerName string
			localIP  string
			ifName   string
		)
		if link.A == node.Name {
			peerName = link.B
			localIP = link.AIP
			ifName = link.AIf
		} else if link.B == node.Name {
			peerName = link.A
			localIP = link.BIP
			ifName = link.BIf
		} else {
			continue
		}

		peerRole := ""
		if peer, ok := nodeMap[peerName]; ok {
			peerRole = peer.Role
		}
		if peerRole == "edge" || strings.HasPrefix(peerName, "edge") {
			if data.EdgeIfName == "" {
				data.EdgeIfName = nodeInterfaceName(node.NodeType, ifName)
			}
			data.Interfaces = append(data.Interfaces, InterfaceData{
				Name: nodeInterfaceName(node.NodeType, ifName),
				Desc: "to " + peerName,
				L2:   true,
				Vlan: 10,
			})
			continue
		}
		if localIP != "" {
			data.Interfaces = append(data.Interfaces, InterfaceData{
				Name: nodeInterfaceName(node.NodeType, ifName),
				IP:   localIP,
				Desc: "to " + peerName,
			})
		}
	}

	data.Protocols = strings.Join(node.Protocols, ", ")
	data.OSPF = protocolRouterID(node.Protocols, "ospf", node.Loopback, "1.1.1.1")
	data.ISIS = protocolNet(node.Protocols, "isis", "49.0001.0000.0000.0001.00")
	data.VXLAN = containsProtocol(node.Protocols, "vxlan") && node.EdgeIP != ""
	if data.VXLAN {
		data.VlanID = 10
		data.Vni = 1010
		data.EdgeIP = node.EdgeIP
		data.EdgePrefix = node.EdgePrefix
	}
	data.MPLS = MPLSData{
		LDP:  containsProtocol(node.Protocols, "mpls-ldp"),
		RSVP: containsProtocol(node.Protocols, "mpls-rsvp"),
	}

	if containsProtocol(node.Protocols, "bgp") {
		data.BGP.Enabled = true
		data.BGP.ASN = node.ASN
		data.BGP.EVPN = containsProtocol(node.Protocols, "evpn")
		if node.Loopback != "" {
			data.BGP.RouterID = node.Loopback
		} else {
			data.BGP.RouterID = "1.1.1.1"
		}
		for _, link := range links {
			var peerName, peerIP string
			if link.A == node.Name {
				peerName = link.B
				peerIP = link.BIP
			} else if link.B == node.Name {
				peerName = link.A
				peerIP = link.AIP
			} else {
				continue
			}
			if peerIP == "" {
				continue
			}
			peerASN := 65000
			if peer, ok := nodeMap[peerName]; ok {
				peerASN = peer.ASN
			}
			data.BGP.Neighbors = append(data.BGP.Neighbors, NeighborData{
				IP:          peerIP,
				ASN:         peerASN,
				Description: peerName,
			})
		}
	}

	tpl, err := template.ParseFiles(filepath.Clean(tplPath))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func RenderContainerlabYAML(labName string, model labplanner.TopologyModel, links []labplanner.LinkAssigned, edgeHosts []labplanner.EdgeHost, monitoring bool) string {
	var b strings.Builder
	b.WriteString("name: " + labName + "\n")
	b.WriteString("topology:\n")
	b.WriteString("  nodes:\n")
	for _, node := range model.Nodes {
		b.WriteString("    " + node.Name + ":\n")
		if strings.EqualFold(node.NodeType, "frr") {
			b.WriteString("      kind: linux\n")
			b.WriteString("      image: quay.io/frrouting/frr:9.1.3\n")
			b.WriteString("      binds:\n")
			b.WriteString("        - configs/" + node.Name + ".cfg:/etc/frr/frr.conf\n")
			b.WriteString("        - configs/" + node.Name + ".daemons:/etc/frr/daemons\n")
		} else {
			b.WriteString("      kind: ceos\n")
			b.WriteString("      image: ceosimage:4.34.2.1f\n")
			b.WriteString("      startup-config: configs/" + node.Name + ".cfg\n")
		}
	}
	for _, host := range edgeHosts {
		b.WriteString("    " + host.Name + ":\n")
		b.WriteString("      kind: linux\n")
		b.WriteString("      image: alpine:3.19\n")
		if host.IP != "" {
			b.WriteString("      exec:\n")
			b.WriteString("        - ip link set " + host.IfName + " up\n")
			b.WriteString("        - ip addr add " + host.IP + "/" + strconv.Itoa(host.Prefix) + " dev " + host.IfName + "\n")
		}
	}
	if monitoring {
		b.WriteString("    prometheus:\n")
		b.WriteString("      kind: linux\n")
		b.WriteString("      image: prom/prometheus:v2.52.0\n")
		b.WriteString("      ports:\n")
		b.WriteString("        - 9090:9090\n")
		b.WriteString("      binds:\n")
		b.WriteString("        - monitoring/prometheus.yml:/etc/prometheus/prometheus.yml\n")
		b.WriteString("    grafana:\n")
		b.WriteString("      kind: linux\n")
		b.WriteString("      image: grafana/grafana:10.4.2\n")
		b.WriteString("      ports:\n")
		b.WriteString("        - 3000:3000\n")
		b.WriteString("      binds:\n")
		b.WriteString("        - monitoring/grafana-datasources.yml:/etc/grafana/provisioning/datasources/datasources.yml\n")
		b.WriteString("    snmp-exporter:\n")
		b.WriteString("      kind: linux\n")
		b.WriteString("      image: prom/snmp-exporter:v0.26.0\n")
		b.WriteString("      binds:\n")
		b.WriteString("        - monitoring/snmp.yml:/etc/snmp_exporter/snmp.yml\n")
		b.WriteString("    gnmic:\n")
		b.WriteString("      kind: linux\n")
		b.WriteString("      image: ghcr.io/openconfig/gnmic:latest\n")
		b.WriteString("      binds:\n")
		b.WriteString("        - monitoring/gnmic.yml:/gnmic/gnmic.yml\n")
		b.WriteString("      cmd: subscribe --config /gnmic/gnmic.yml\n")
	}
	b.WriteString("  links:\n")
	for _, link := range links {
		b.WriteString("    - endpoints: [" + link.A + ":" + link.AIf + ", " + link.B + ":" + link.BIf + "]\n")
	}
	return b.String()
}

func nodeInterfaceName(nodeType, name string) string {
	if strings.EqualFold(nodeType, "frr") {
		return name
	}
	if strings.HasPrefix(name, "eth") {
		return "Ethernet" + strings.TrimPrefix(name, "eth")
	}
	return name
}

func containsProtocol(list []string, target string) bool {
	for _, item := range list {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func protocolRouterID(protocols []string, target, loopback, fallback string) string {
	if !containsProtocol(protocols, target) {
		return ""
	}
	if loopback != "" {
		return loopback
	}
	return fallback
}

func protocolNet(protocols []string, target, fallback string) string {
	if !containsProtocol(protocols, target) {
		return ""
	}
	return fallback
}

func RenderFRRDaemons(protocols []string) string {
	var b strings.Builder
	b.WriteString("zebra=yes\n")
	b.WriteString("bgpd=" + yesNo(containsProtocol(protocols, "bgp")) + "\n")
	b.WriteString("ospfd=" + yesNo(containsProtocol(protocols, "ospf")) + "\n")
	b.WriteString("isisd=" + yesNo(containsProtocol(protocols, "isis")) + "\n")
	b.WriteString("ldpd=" + yesNo(containsProtocol(protocols, "mpls-ldp")) + "\n")
	b.WriteString("pimd=no\n")
	b.WriteString("ripd=no\n")
	b.WriteString("ripngd=no\n")
	b.WriteString("ospf6d=no\n")
	b.WriteString("eigrpd=no\n")
	b.WriteString("babeld=no\n")
	b.WriteString("sharpd=no\n")
	b.WriteString("pbrd=no\n")
	b.WriteString("bfdd=no\n")
	b.WriteString("fabricd=no\n")
	b.WriteString("vrrpd=no\n")
	b.WriteString("pathd=no\n")
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func PrometheusConfig(labName string, snmpEnabled, gnmiEnabled bool) string {
	var b strings.Builder
	b.WriteString("global:\n  scrape_interval: 15s\n")
	b.WriteString("scrape_configs:\n")
	if snmpEnabled {
		b.WriteString("  - job_name: \"snmp\"\n")
		b.WriteString("    metrics_path: /snmp\n")
		b.WriteString("    params:\n")
		b.WriteString("      module: [\"eos\"]\n")
		b.WriteString("      auth: [\"public_v2\"]\n")
		b.WriteString("    static_configs:\n")
		b.WriteString("      - targets:\n")
		b.WriteString("          - clab-" + labName + "-leaf1\n")
		b.WriteString("    relabel_configs:\n")
		b.WriteString("      - source_labels: [__address__]\n")
		b.WriteString("        target_label: __param_target\n")
		b.WriteString("      - source_labels: [__param_target]\n")
		b.WriteString("        target_label: instance\n")
		b.WriteString("      - target_label: __address__\n")
		b.WriteString("        replacement: snmp-exporter:9116\n")
	}
	if gnmiEnabled {
		b.WriteString("  - job_name: \"gnmi\"\n")
		b.WriteString("    static_configs:\n")
		b.WriteString("      - targets:\n")
		b.WriteString("          - gnmic:9804\n")
	}
	return b.String()
}

func SNMPConfig() string {
	return `
auths:
  public_v2:
    version: 2
    community: public
modules:
  eos:
    walk:
      - 1.3.6.1.2.1.2
      - 1.3.6.1.2.1.31
    metrics:
      - name: ifHCInOctets
        oid: 1.3.6.1.2.1.31.1.1.1.6
        type: counter64
        help: "Total octets received on the interface."
        indexes:
          - labelname: ifIndex
            type: gauge
        lookups:
          - labels: [ifIndex]
            labelname: ifName
            oid: 1.3.6.1.2.1.31.1.1.1.1
            type: DisplayString
      - name: ifHCOutOctets
        oid: 1.3.6.1.2.1.31.1.1.1.10
        type: counter64
        help: "Total octets transmitted on the interface."
        indexes:
          - labelname: ifIndex
            type: gauge
        lookups:
          - labels: [ifIndex]
            labelname: ifName
            oid: 1.3.6.1.2.1.31.1.1.1.1
            type: DisplayString
      - name: ifInErrors
        oid: 1.3.6.1.2.1.2.2.1.14
        type: counter
        help: "Inbound errors on the interface."
        indexes:
          - labelname: ifIndex
            type: gauge
        lookups:
          - labels: [ifIndex]
            labelname: ifName
            oid: 1.3.6.1.2.1.31.1.1.1.1
            type: DisplayString
      - name: ifOutErrors
        oid: 1.3.6.1.2.1.2.2.1.20
        type: counter
        help: "Outbound errors on the interface."
        indexes:
          - labelname: ifIndex
            type: gauge
        lookups:
          - labels: [ifIndex]
            labelname: ifName
            oid: 1.3.6.1.2.1.31.1.1.1.1
            type: DisplayString
`
}

func GrafanaDatasource() string {
	return `
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
`
}

func GNMIConfig(labName string) string {
	return `
targets:
  leaf1:
    address: clab-` + labName + `-leaf1:6030
    username: admin
    password: admin
    insecure: true
subscriptions:
  interfaces:
    path: /interfaces/interface/state/counters
    stream-mode: sample
    sample-interval: 10s
outputs:
  prometheus:
    type: prometheus
    listen: 0.0.0.0:9804
`
}
