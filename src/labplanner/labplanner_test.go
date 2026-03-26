package labplanner

import "testing"

func TestEdgeLinks_DefaultsToDistinctTargetsPerEdge(t *testing.T) {
	model := TopologyModel{
		Nodes: []TopologyNode{
			{Name: "spine1", Role: "spine"},
			{Name: "leaf1", Role: "leaf"},
			{Name: "leaf2", Role: "leaf"},
			{Name: "leaf3", Role: "leaf"},
		},
		EdgeFanout: 2,
	}

	links := EdgeLinks(model, []string{"edge1"}, nil)
	if len(links) != 2 {
		t.Fatalf("expected 2 edge links, got %d", len(links))
	}
	if links[0].B == links[1].B {
		t.Fatalf("expected distinct targets, got %#v", links)
	}
}

func TestEdgeLinks_HonorsPreferredAttachmentBeforeAutoFill(t *testing.T) {
	model := TopologyModel{
		Nodes: []TopologyNode{
			{Name: "leaf1", Role: "leaf"},
			{Name: "leaf2", Role: "leaf"},
			{Name: "leaf3", Role: "leaf"},
		},
		EdgeFanout: 2,
	}

	links := EdgeLinks(model, []string{"edge1"}, []EdgeAttachment{{Edge: "edge1", Target: "leaf3"}})
	if len(links) != 2 {
		t.Fatalf("expected 2 edge links, got %d", len(links))
	}
	if links[0].B != "leaf3" {
		t.Fatalf("expected explicit target first, got %#v", links)
	}
	if links[1].B == "leaf3" {
		t.Fatalf("expected auto-filled target to be distinct, got %#v", links)
	}
}

func TestEdgeLinks_MultiHomingMovesEdge2ToLeaf3ByDefault(t *testing.T) {
	model := TopologyModel{
		Topology: "leaf-spine",
		Nodes: []TopologyNode{
			{Name: "leaf1", Role: "leaf"},
			{Name: "leaf2", Role: "leaf"},
			{Name: "leaf3", Role: "leaf"},
		},
		EdgeFanout: 2,
	}

	links := EdgeLinks(model, []string{"edge1", "edge2"}, nil)
	if len(links) != 3 {
		t.Fatalf("expected 3 edge links, got %d", len(links))
	}
	if links[0].A != "edge1" || links[0].B != "leaf1" || links[1].A != "edge1" || links[1].B != "leaf2" {
		t.Fatalf("expected edge1 to connect to leaf1 and leaf2, got %#v", links[:2])
	}
	if links[2].A != "edge2" || links[2].B != "leaf3" {
		t.Fatalf("expected edge2 to default to leaf3, got %#v", links[2])
	}
}

func TestBuildLabPlan_MultiHomedEdgeHostTracksAllInterfaces(t *testing.T) {
	model := TopologyModel{
		Topology:   "leaf-spine",
		EdgeNodes:  1,
		EdgeFanout: 2,
		Nodes: []TopologyNode{
			{Name: "spine1", Role: "spine", NodeType: "frr"},
			{Name: "leaf1", Role: "leaf", NodeType: "frr"},
			{Name: "leaf2", Role: "leaf", NodeType: "frr"},
		},
		Links: []TopologyLink{
			{A: "spine1", B: "leaf1"},
			{A: "spine1", B: "leaf2"},
		},
	}

	plan, err := BuildLabPlan("10.0.0.0/24", "172.16.0.0/24", model, nil)
	if err != nil {
		t.Fatalf("build lab plan: %v", err)
	}
	if len(plan.EdgeHosts) != 1 {
		t.Fatalf("expected 1 edge host, got %d", len(plan.EdgeHosts))
	}
	if len(plan.EdgeHosts[0].IfNames) != 2 {
		t.Fatalf("expected multi-homed edge host interfaces, got %#v", plan.EdgeHosts[0].IfNames)
	}
}

func TestBuildLabPlan_SecondaryEdgesRemainSingleHomed(t *testing.T) {
	model := TopologyModel{
		Topology:   "leaf-spine",
		EdgeNodes:  2,
		EdgeFanout: 2,
		Nodes: []TopologyNode{
			{Name: "spine1", Role: "spine", NodeType: "frr"},
			{Name: "leaf1", Role: "leaf", NodeType: "frr"},
			{Name: "leaf2", Role: "leaf", NodeType: "frr"},
			{Name: "leaf3", Role: "leaf", NodeType: "frr"},
		},
		Links: []TopologyLink{
			{A: "spine1", B: "leaf1"},
			{A: "spine1", B: "leaf2"},
			{A: "spine1", B: "leaf3"},
		},
	}

	plan, err := BuildLabPlan("10.0.0.0/24", "172.16.0.0/24", model, nil)
	if err != nil {
		t.Fatalf("build lab plan: %v", err)
	}
	if len(plan.EdgeHosts) != 2 {
		t.Fatalf("expected 2 edge hosts, got %d", len(plan.EdgeHosts))
	}
	if len(plan.EdgeHosts[0].IfNames) != 2 {
		t.Fatalf("expected edge1 to stay multi-homed, got %#v", plan.EdgeHosts[0].IfNames)
	}
	if len(plan.EdgeHosts[1].IfNames) != 1 || plan.EdgeHosts[1].IfNames[0] != "eth1" {
		t.Fatalf("expected edge2 to stay single-homed, got %#v", plan.EdgeHosts[1].IfNames)
	}
}
