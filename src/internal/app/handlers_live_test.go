package app

import (
	"testing"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
)

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

func TestPeeringKey_IsOrderIndependent(t *testing.T) {
	a := LivePeeringStatus{A: "leaf1", B: "spine1", AfiSafi: "ipv4/unicast"}
	b := LivePeeringStatus{A: "spine1", B: "leaf1", AfiSafi: "ipv4/unicast"}
	if peeringKey(a) != peeringKey(b) {
		t.Fatalf("expected order-independent key, got %q vs %q", peeringKey(a), peeringKey(b))
	}
}

func TestPeeringFromSummary_UsesEstablishedState(t *testing.T) {
	p := frrPeerSummary{Peer: "10.0.0.1", State: "Established", Established: true}
	entry := peeringFromSummary("leaf1", "spine1", "l2vpn/evpn", p)
	if entry.State != "up" {
		t.Fatalf("expected up state, got %q", entry.State)
	}
	if entry.AfiSafi != "l2vpn/evpn" {
		t.Fatalf("expected afi/safi label, got %q", entry.AfiSafi)
	}
}

func TestReconcilePeeringsWithLinks_DirectDownWins(t *testing.T) {
	peerings := []LivePeeringStatus{
		{A: "leaf1", B: "spine1", AfiSafi: "ipv4/unicast", State: "up", Detail: "Established"},
	}
	links := []LiveLinkStatus{
		{A: "leaf1", B: "spine1", State: "down"},
	}
	out := reconcilePeeringsWithLinks(peerings, links)
	if len(out) != 1 {
		t.Fatalf("expected one peering, got %d", len(out))
	}
	if out[0].State != "down" {
		t.Fatalf("expected down after direct link failure, got %q", out[0].State)
	}
}

func TestPeerIPNodeMap_IncludesLoopbacksAndLinkIPs(t *testing.T) {
	nodes := []labplanner.NodePlan{
		{Name: "leaf1", Loopback: "10.0.0.3"},
		{Name: "spine1", Loopback: "10.0.0.1"},
	}
	links := []labplanner.LinkAssigned{
		{A: "leaf1", AIP: "10.0.0.7", B: "spine1", BIP: "10.0.0.6"},
	}
	m := peerIPNodeMap(nodes, links)
	if m["10.0.0.3"] != "leaf1" || m["10.0.0.1"] != "spine1" {
		t.Fatalf("expected loopback mapping, got %#v", m)
	}
	if m["10.0.0.7"] != "leaf1" || m["10.0.0.6"] != "spine1" {
		t.Fatalf("expected p2p link mapping, got %#v", m)
	}
}
