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
