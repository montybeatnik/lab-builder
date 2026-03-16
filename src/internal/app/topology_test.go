package app

import "testing"

func TestBuildTopologyModel_NodeTypeFRR(t *testing.T) {
	model, errs, _ := BuildTopologyModel(TopologyRequest{
		Topology:   "leaf-spine",
		NodeType:   "frr",
		SpineCount: 1,
		LeafCount:  2,
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, n := range model.Nodes {
		if n.NodeType != "frr" {
			t.Fatalf("expected all nodes to be frr, got %s for %s", n.NodeType, n.Name)
		}
	}
}

func TestBuildTopologyModel_InvalidNodeType(t *testing.T) {
	_, errs, _ := BuildTopologyModel(TopologyRequest{
		Topology:   "leaf-spine",
		NodeType:   "not-real",
		SpineCount: 1,
		LeafCount:  1,
	})
	if len(errs) == 0 {
		t.Fatal("expected validation error for invalid node type")
	}
}
