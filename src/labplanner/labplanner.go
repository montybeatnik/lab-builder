package labplanner

import (
	"net"
	"strings"
)

type TopologyNode struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	NodeType string `json:"nodeType,omitempty"`
}

type TopologyLink struct {
	A string `json:"a"`
	B string `json:"b"`
}

type ProtocolSet struct {
	Global []string            `json:"global"`
	Roles  map[string][]string `json:"roles"`
}

type TopologyModel struct {
	Topology  string         `json:"topology"`
	Nodes     []TopologyNode `json:"nodes"`
	Links     []TopologyLink `json:"links"`
	EdgeNodes int            `json:"edgeNodes"`
	Protocols ProtocolSet    `json:"protocols"`
}

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

type LinkAssigned struct {
	A      string
	B      string
	AIf    string
	BIf    string
	Subnet string
	AIP    string
	BIP    string
}

type LabPlan struct {
	Nodes     []NodePlan
	Links     []LinkAssigned
	EdgeHosts []EdgeHost
}

type EdgeAttachment struct {
	Edge   string
	Target string
}

type EdgeHost struct {
	Name   string
	IP     string
	Prefix int
	IfName string
}

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

func ProtocolsForRole(role string, protocols ProtocolSet) []string {
	list := append([]string{}, protocols.Global...)
	if roleList, ok := protocols.Roles[role]; ok {
		list = append(list, roleList...)
	}
	return uniqueStrings(list)
}

func ContainsProtocol(list []string, target string) bool {
	for _, item := range list {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func EdgeNodeNames(count int) []string {
	var names []string
	for i := 1; i <= count; i++ {
		names = append(names, "edge"+itoa(i))
	}
	return names
}

func EdgeLinks(model TopologyModel, edges []string, attachments []EdgeAttachment) []TopologyLink {
	if len(edges) == 0 {
		return nil
	}
	attachMap := map[string]string{}
	for _, a := range attachments {
		attachMap[a.Edge] = a.Target
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
	var links []TopologyLink
	for i, edge := range edges {
		target := targets[i%len(targets)]
		if desired, ok := attachMap[edge]; ok && validTargets[desired] {
			target = desired
		}
		links = append(links, TopologyLink{A: edge, B: target})
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
		ifName := edgeIfName(name, links)
		hosts = append(hosts, EdgeHost{
			Name:   name,
			IP:     intToIP(cur),
			Prefix: prefix,
			IfName: ifName,
		})
		cur++
	}
	return hosts, nil
}

func edgeIfName(name string, links []LinkAssigned) string {
	for _, link := range links {
		if link.A == name {
			return link.AIf
		}
		if link.B == name {
			return link.BIf
		}
	}
	return "eth1"
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
