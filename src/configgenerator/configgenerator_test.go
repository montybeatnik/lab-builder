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
	body := RenderContainerlabYAML("mixed-lab", model, nil, nil, false)

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
