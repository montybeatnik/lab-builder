package configgenerator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
)

func TestRenderContainerlabYAML_NodeTypes(t *testing.T) {
	model := labplanner.TopologyModel{
		Nodes: []labplanner.TopologyNode{
			{Name: "spine1", Role: "spine", NodeType: "arista"},
			{Name: "leaf1", Role: "leaf", NodeType: "frr"},
		},
	}
	body := RenderContainerlabYAML("mixed-lab", model, nil, nil, nil, false)

	if !strings.Contains(body, "spine1:\n      kind: ceos") {
		t.Fatal("expected arista node to render as ceos")
	}
	if !strings.Contains(body, "leaf1:\n      kind: linux") {
		t.Fatal("expected frr node to render as linux")
	}
	if !strings.Contains(body, "configs/leaf1.daemons:/etc/frr/daemons") {
		t.Fatal("expected frr daemons bind in output")
	}
}

func TestRenderContainerlabYAML_FRRVXLANExec(t *testing.T) {
	model := labplanner.TopologyModel{
		Nodes: []labplanner.TopologyNode{
			{Name: "leaf1", Role: "leaf", NodeType: "frr"},
			{Name: "leaf2", Role: "leaf", NodeType: "frr"},
		},
	}
	nodes := []labplanner.NodePlan{
		{Name: "leaf1", Role: "leaf", NodeType: "frr", Loopback: "10.0.0.3", Protocols: []string{"bgp", "evpn", "vxlan"}},
		{Name: "leaf2", Role: "leaf", NodeType: "frr", Loopback: "10.0.0.4", Protocols: []string{"bgp", "evpn", "vxlan"}},
	}
	links := []labplanner.LinkAssigned{
		{A: "leaf1", AIf: "eth3", B: "edge1", BIf: "eth1"},
		{A: "leaf2", AIf: "eth3", B: "edge1", BIf: "eth2"},
		{A: "leaf1", AIf: "eth4", B: "edge2", BIf: "eth1"},
	}

	body := RenderContainerlabYAML("vxlan-lab", model, nodes, links, nil, false)

	if !strings.Contains(body, "exec:\n        - ip link add vxlan10 type vxlan id 10 local 10.0.0.3 dstport 4789 nolearning") {
		t.Fatal("expected vxlan startup exec for frr leaf")
	}
	if !strings.Contains(body, "ip link add bond0 type bond mode active-backup") {
		t.Fatal("expected bond0 creation for EVPN multihoming leaf")
	}
	if !strings.Contains(body, "ip link set dev eth3 master bond0") {
		t.Fatal("expected edge1-facing interface to join bond0")
	}
	if !strings.Contains(body, "ip link set dev bond0 master br0") {
		t.Fatal("expected bond0 to join br0")
	}
	if !strings.Contains(body, "ip link set dev eth4 master br0") {
		t.Fatal("expected standalone edge interfaces to join br0")
	}
	if !strings.Contains(body, "bridge fdb append 00:00:00:00:00:00 dev vxlan10 dst 10.0.0.4") {
		t.Fatal("expected remote VTEP flood entry for FRR vxlan leaf")
	}
}

func TestRenderContainerlabYAML_MultiHomedEdgeHostUsesBond(t *testing.T) {
	model := labplanner.TopologyModel{}
	edgeHosts := []labplanner.EdgeHost{
		{Name: "edge1", IP: "172.16.0.10", Prefix: 24, IfNames: []string{"eth1", "eth2"}},
	}

	body := RenderContainerlabYAML("fanout-lab", model, nil, nil, edgeHosts, false)

	if !strings.Contains(body, "ip link add bond0 type bond mode active-backup") {
		t.Fatal("expected edge bond creation in output")
	}
	if !strings.Contains(body, "ip link set dev eth1 master bond0") || !strings.Contains(body, "ip link set dev eth2 master bond0") {
		t.Fatal("expected all edge interfaces to join the bond")
	}
	if !strings.Contains(body, "ip addr add 172.16.0.10/24 dev bond0") {
		t.Fatal("expected edge IP to be assigned to bond0")
	}
}

func TestRenderNodeConfig_EVPNMHUsesBondInterface(t *testing.T) {
	node := labplanner.NodePlan{Name: "leaf1", Role: "leaf", NodeType: "frr", Loopback: "10.0.0.2", Protocols: []string{"bgp", "evpn", "vxlan"}}
	allLinks := []labplanner.LinkAssigned{
		{A: "edge1", AIf: "eth1", B: "leaf1", BIf: "eth2"},
		{A: "edge1", AIf: "eth2", B: "leaf2", BIf: "eth2"},
		{A: "leaf1", AIf: "eth1", B: "spine1", BIf: "eth1", AIP: "10.0.0.7", BIP: "10.0.0.6"},
	}
	nodeLinks := []labplanner.LinkAssigned{
		allLinks[0],
		allLinks[2],
	}
	nodeMap := map[string]labplanner.NodePlan{
		"leaf1":  node,
		"spine1": {Name: "spine1", Role: "spine", ASN: 65000},
	}

	body, err := RenderNodeConfig(filepath.Join("..", "templates", "config", "node_frr.tmpl"), node, nodeLinks, allLinks, nodeMap, false, false)
	if err != nil {
		t.Fatalf("render config: %v", err)
	}
	if !strings.Contains(body, "interface bond0") {
		t.Fatal("expected bond0 interface in FRR config")
	}
	if !strings.Contains(body, "evpn mh es-id 1") || !strings.Contains(body, "evpn mh es-sys-mac 02:00:00:00:00:01") {
		t.Fatal("expected EVPN multihoming config on bond0")
	}
	if strings.Contains(body, "interface eth2\n description to edge1") {
		t.Fatal("did not expect edge1 member interface to be rendered as a standalone access port")
	}
}
