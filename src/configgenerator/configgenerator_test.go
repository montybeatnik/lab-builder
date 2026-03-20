package configgenerator

import (
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
		},
	}
	nodes := []labplanner.NodePlan{
		{Name: "leaf1", Role: "leaf", NodeType: "frr", Loopback: "10.0.0.3", Protocols: []string{"bgp", "evpn", "vxlan"}},
	}
	links := []labplanner.LinkAssigned{
		{A: "leaf1", AIf: "eth3", B: "edge1", BIf: "eth1"},
	}

	body := RenderContainerlabYAML("vxlan-lab", model, nodes, links, nil, false)

	if !strings.Contains(body, "exec:\n        - ip link add vxlan10 type vxlan id 10 local 10.0.0.3 dev eth3 dstport 4789") {
		t.Fatal("expected vxlan startup exec for frr leaf")
	}
	if !strings.Contains(body, "ip link set dev eth3 master br0") {
		t.Fatal("expected bridge membership for edge-facing interface")
	}
}
