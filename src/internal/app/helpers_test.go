package app

import "testing"

func TestCIDRIP(t *testing.T) {
	if got := CIDRIP("172.20.20.7/24"); got != "172.20.20.7" {
		t.Fatalf("expected ip got %q", got)
	}
	if got := CIDRIP("10.0.0.1"); got != "10.0.0.1" {
		t.Fatalf("expected ip got %q", got)
	}
}

func TestOnlyNonEmpty(t *testing.T) {
	in := []string{"a", "  ", "b", "", " c "}
	out := OnlyNonEmpty(in)
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestCeosNodesFromInspect(t *testing.T) {
	payload := []byte(`{"lab1":[{"kind":"ceos","ipv4_address":"10.0.0.1/24","name":"leaf1"},{"kind":"linux","ipv4_address":"10.0.0.2/24","name":"host1"},{"kind":"ceos","ipv4_address":"","name":"leaf2"}]}`)
	out, err := CEOSNodesFromInspect(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 node got %d", len(out))
	}
	if out[0].Name != "leaf1" {
		t.Fatalf("unexpected node: %#v", out[0])
	}
}

func TestHealthNodesFromInspect_IncludesFRR(t *testing.T) {
	payload := []byte(`{"lab1":[{"kind":"ceos","image":"ceos:latest","ipv4_address":"10.0.0.1/24","name":"leaf1"},{"kind":"linux","image":"quay.io/frrouting/frr:9.1.3","ipv4_address":"172.20.20.3/24","container_id":"abc123","name":"spine1"},{"kind":"linux","image":"alpine:3.19","ipv4_address":"172.20.20.4/24","name":"host1"}]}`)
	out, err := healthNodesFromInspect(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 supported nodes got %d", len(out))
	}
	if out[1].Name != "spine1" {
		t.Fatalf("unexpected FRR node: %#v", out[1])
	}
}

func TestExtractFRRPeerSummaries(t *testing.T) {
	data := map[string]any{
		"ipv4Unicast": map[string]any{
			"peers": map[string]any{
				"10.0.0.6": map[string]any{
					"state":  "Established",
					"pfxRcd": float64(2),
					"pfxSnt": float64(1),
				},
			},
		},
	}
	peers := extractFRRPeerSummaries(data)
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer got %d", len(peers))
	}
	if !peers[0].Established || peers[0].PfxRcd != 2 || peers[0].PfxSnt != 1 {
		t.Fatalf("unexpected parsed peer: %#v", peers[0])
	}
}
