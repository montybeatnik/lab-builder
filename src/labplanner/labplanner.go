package labplanner

import (
	"net"
	"strings"
)

// TopologyNode is a user-selected node before planner-derived attributes are assigned.
type TopologyNode struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	NodeType string `json:"nodeType,omitempty"`
}

// TopologyLink is an intent-level edge between two named nodes before interface/IP assignment.
type TopologyLink struct {
	A string `json:"a"`
	B string `json:"b"`
}

// ProtocolSet carries protocol intent globally and by role so generation stays role-driven.
type ProtocolSet struct {
	Global []string            `json:"global"`
	Roles  map[string][]string `json:"roles"`
}

// TopologyModel is the validated UI request normalized into a planner-friendly topology graph.
type TopologyModel struct {
	Topology   string         `json:"topology"`
	Nodes      []TopologyNode `json:"nodes"`
	Links      []TopologyLink `json:"links"`
	EdgeNodes  int            `json:"edgeNodes"`
	EdgeFanout int            `json:"edgeFanout"`
	Protocols  ProtocolSet    `json:"protocols"`
}

// NodePlan is the per-node derived plan used by config and topology renderers.
type NodePlan struct {
	Name       string
	Role       string
	NodeType   string
	ASN        int
	Loopback   string
	EdgeIP     string
	EdgePrefix int
	Protocols  []string
}

// LinkAssigned is a topology link with concrete interface names and addressing.
type LinkAssigned struct {
	A      string
	B      string
	AIf    string
	BIf    string
	Subnet string
	AIP    string
	BIP    string
}

// LabPlan is the full computed output consumed by file generation and deployment handlers.
type LabPlan struct {
	Nodes     []NodePlan
	Links     []LinkAssigned
	EdgeHosts []EdgeHost
}

// EdgeAttachment lets callers pin specific edge hosts to target fabric nodes.
type EdgeAttachment struct {
	Edge   string
	Target string
}

// EdgeHost describes generated edge endpoint addressing and uplink interfaces.
type EdgeHost struct {
	Name    string
	IP      string
	Prefix  int
	IfNames []string
}

// BuildLabPlan converts a validated topology model into deployable node/link/address assignments.
func BuildLabPlan(infraCIDR, edgeCIDR string, model TopologyModel, attachments []EdgeAttachment) (LabPlan, error) {
	nodes := assignASNs(model.Nodes, model.Protocols)
	allLinks := append([]TopologyLink{}, model.Links...)
	allLinks = append(allLinks, EdgeLinks(model, EdgeNodeNames(model.EdgeNodes), attachments)...)
	assigned := assignInterfaces(allLinks)

	loopbacks, linksWithIPs, err := allocateInfraIPs(infraCIDR, nodes, assigned)
	if err != nil {
		return LabPlan{}, err
	}
	for i := range nodes {
		if loop, ok := loopbacks[nodes[i].Name]; ok {
			nodes[i].Loopback = loop
		}
	}

	edgeHosts, err := allocateEdgeIPs(edgeCIDR, nodes, model.EdgeNodes, assigned)
	if err != nil {
		return LabPlan{}, err
	}

	return LabPlan{
		Nodes:     nodes,
		Links:     linksWithIPs,
		EdgeHosts: edgeHosts,
	}, nil
}

// NormalizeProtocols deduplicates/normalizes protocol intent so later stages can trust it.
func NormalizeProtocols(p ProtocolSet) ProtocolSet {
	out := ProtocolSet{
		Global: uniqueStrings(p.Global),
		Roles:  map[string][]string{},
	}
	for role, list := range p.Roles {
		key := strings.ToLower(strings.TrimSpace(role))
		if key == "" {
			continue
		}
		out.Roles[key] = uniqueStrings(list)
	}
	return out
}

// ProtocolsForRole resolves role-specific protocols layered on top of global defaults.
func ProtocolsForRole(role string, protocols ProtocolSet) []string {
	list := append([]string{}, protocols.Global...)
	if roleList, ok := protocols.Roles[role]; ok {
		list = append(list, roleList...)
	}
	return uniqueStrings(list)
}

// ContainsProtocol performs case-insensitive membership checks for protocol feature switches.
func ContainsProtocol(list []string, target string) bool {
	for _, item := range list {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

// EdgeNodeNames provides stable generated edge host names used across planner and templates.
func EdgeNodeNames(count int) []string {
	var names []string
	for i := 1; i <= count; i++ {
		names = append(names, "edge"+itoa(i))
	}
	return names
}

// EdgeLinks computes edge-to-fabric attachment links including optional multi-homing fanout.
func EdgeLinks(model TopologyModel, edges []string, attachments []EdgeAttachment) []TopologyLink {
	if len(edges) == 0 {
		return nil
	}
	attachMap := map[string][]string{}
	for _, a := range attachments {
		if strings.TrimSpace(a.Edge) == "" || strings.TrimSpace(a.Target) == "" {
			continue
		}
		attachMap[a.Edge] = append(attachMap[a.Edge], a.Target)
	}
	var targets []string
	validTargets := map[string]bool{}
	for _, n := range model.Nodes {
		if n.Role == "leaf" || n.Role == "spoke" || n.Role == "mesh" || n.Role == "custom" {
			targets = append(targets, n.Name)
			validTargets[n.Name] = true
		}
	}
	if len(targets) == 0 {
		for _, n := range model.Nodes {
			targets = append(targets, n.Name)
			validTargets[n.Name] = true
		}
	}
	fanout := model.EdgeFanout
	if fanout < 1 {
		fanout = 1
	}
	var links []TopologyLink
	for i, edge := range edges {
		seen := map[string]bool{}
		var chosen []string
		startOffset := i
		desiredFanout := fanout
		if model.Topology == "leaf-spine" && fanout > 1 && edge != "edge1" {
			// Multi-homing is intentionally modeled only for edge1. Other edge hosts
			// remain single-homed unless explicitly extended in the future.
			desiredFanout = 1
		}
		if model.Topology == "leaf-spine" && fanout > 1 && len(targets) > fanout-1 && i > 0 {
			// When edge1 is multi-homed to the first two leaves, start subsequent
			// default edge attachments after that reserved pair.
			startOffset = i + fanout - 1
		}

		// Honor any explicit targets first, then fill the remaining slots from the
		// eligible target pool so each edge attaches to distinct leaves.
		for _, desired := range attachMap[edge] {
			if !validTargets[desired] || seen[desired] {
				continue
			}
			seen[desired] = true
			chosen = append(chosen, desired)
		}
		for offset := 0; len(chosen) < desiredFanout && offset < len(targets); offset++ {
			target := targets[(startOffset+offset)%len(targets)]
			if seen[target] {
				continue
			}
			seen[target] = true
			chosen = append(chosen, target)
		}
		for _, target := range chosen {
			links = append(links, TopologyLink{A: edge, B: target})
		}
	}
	return links
}

func assignASNs(nodes []TopologyNode, protocols ProtocolSet) []NodePlan {
	var plans []NodePlan
	roleCounters := map[string]int{}
	for _, n := range nodes {
		role := strings.ToLower(n.Role)
		roleCounters[role]++
		asn := 65000
		switch role {
		case "spine", "hub":
			asn = 65000
		case "leaf", "spoke":
			asn = 65100 + roleCounters[role] - 1
		case "mesh", "custom":
			asn = 65200 + roleCounters[role] - 1
		default:
			asn = 65300 + roleCounters[role] - 1
		}
		plans = append(plans, NodePlan{
			Name:      n.Name,
			Role:      role,
			NodeType:  normalizeNodeType(n.NodeType),
			ASN:       asn,
			Protocols: ProtocolsForRole(role, protocols),
		})
	}
	return plans
}

func normalizeNodeType(nodeType string) string {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "frr":
		return "frr"
	default:
		return "arista"
	}
}

func assignInterfaces(links []TopologyLink) []LinkAssigned {
	ifIndex := map[string]int{}
	nextIf := func(node string) string {
		ifIndex[node]++
		return "eth" + itoa(ifIndex[node])
	}
	var out []LinkAssigned
	for _, l := range links {
		out = append(out, LinkAssigned{
			A:   l.A,
			B:   l.B,
			AIf: nextIf(l.A),
			BIf: nextIf(l.B),
		})
	}
	return out
}

func allocateInfraIPs(infraCIDR string, nodes []NodePlan, links []LinkAssigned) (map[string]string, []LinkAssigned, error) {
	if infraCIDR == "" {
		return nil, links, nil
	}
	base, _, err := parseCIDR(infraCIDR)
	if err != nil {
		return nil, links, err
	}

	nodeMap := map[string]NodePlan{}
	for _, n := range nodes {
		nodeMap[n.Name] = n
	}

	var infraNodes []string
	for _, n := range nodes {
		if n.Role != "edge" {
			infraNodes = append(infraNodes, n.Name)
		}
	}

	cur := base + 1
	loopbacks := map[string]string{}
	for _, name := range infraNodes {
		loopbacks[name] = intToIP(cur)
		cur++
	}

	if cur%2 == 1 {
		cur++
	}

	for i := range links {
		aNode, aOK := nodeMap[links[i].A]
		bNode, bOK := nodeMap[links[i].B]
		if !aOK || !bOK {
			continue
		}
		if aNode.Role == "edge" || bNode.Role == "edge" {
			continue
		}
		subnetBase := cur
		links[i].Subnet = intToIP(subnetBase) + "/31"
		links[i].AIP = intToIP(subnetBase)
		links[i].BIP = intToIP(subnetBase + 1)
		cur += 2
	}
	return loopbacks, links, nil
}

func allocateEdgeIPs(edgeCIDR string, nodes []NodePlan, edgeCount int, links []LinkAssigned) ([]EdgeHost, error) {
	if strings.TrimSpace(edgeCIDR) == "" {
		return nil, nil
	}
	base, prefix, err := parseCIDR(edgeCIDR)
	if err != nil {
		return nil, err
	}
	cur := base + 1
	for i := range nodes {
		if !ContainsProtocol(nodes[i].Protocols, "vxlan") {
			continue
		}
		if nodes[i].Role == "spine" || nodes[i].Role == "hub" {
			continue
		}
		nodes[i].EdgeIP = intToIP(cur)
		nodes[i].EdgePrefix = prefix
		cur++
	}
	var hosts []EdgeHost
	for i := 1; i <= edgeCount; i++ {
		name := "edge" + itoa(i)
		hosts = append(hosts, EdgeHost{
			Name:    name,
			IP:      intToIP(cur),
			Prefix:  prefix,
			IfNames: edgeIfNames(name, links),
		})
		cur++
	}
	return hosts, nil
}

func edgeIfNames(name string, links []LinkAssigned) []string {
	var names []string
	for _, link := range links {
		if link.A == name {
			names = append(names, link.AIf)
		}
		if link.B == name {
			names = append(names, link.BIf)
		}
	}
	if len(names) == 0 {
		return []string{"eth1"}
	}
	return names
}

func parseCIDR(cidr string) (uint32, int, error) {
	ip, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return 0, 0, err
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, 0, ErrInvalidCIDR("only IPv4 CIDRs are supported")
	}
	ones, _ := ipNet.Mask.Size()
	return ipToInt(ip4), ones, nil
}

func ipToInt(ip net.IP) uint32 {
	ip4 := ip.To4()
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

func intToIP(v uint32) string {
	return itoa(int((v>>24)&0xff)) + "." + itoa(int((v>>16)&0xff)) + "." + itoa(int((v>>8)&0xff)) + "." + itoa(int(v&0xff))
}

type ErrInvalidCIDR string

func (e ErrInvalidCIDR) Error() string { return string(e) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func uniqueStrings(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range list {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
