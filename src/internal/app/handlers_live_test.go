package app

import "testing"

func TestCombineLinkState(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{a: "up", b: "up", want: "up"},
		{a: "down", b: "up", want: "down"},
		{a: "up", b: "down", want: "down"},
		{a: "unknown", b: "up", want: "unknown"},
	}
	for _, tc := range tests {
		got := combineLinkState(tc.a, tc.b)
		if got != tc.want {
			t.Fatalf("combineLinkState(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestShortNodeName(t *testing.T) {
	if got := shortNodeName("clab-frr-lab-leaf1", "frr-lab"); got != "leaf1" {
		t.Fatalf("expected leaf1, got %q", got)
	}
	if got := shortNodeName("edge1", "frr-lab"); got != "edge1" {
		t.Fatalf("expected passthrough name, got %q", got)
	}
}
