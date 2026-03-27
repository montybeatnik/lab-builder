package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/montybeatnik/arista-lab/laber/labstore"
	"github.com/montybeatnik/arista-lab/laber/pkgs/arista"
	"github.com/montybeatnik/arista-lab/laber/pkgs/renderer"
)

func runInspect(ctx context.Context, labPath string, useSudo bool) ([]byte, error) {
	args := []string{"containerlab", "inspect", "-t", labPath, "--format", "json"}
	if useSudo {
		args = append([]string{"sudo", "-n"}, args...)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func runContainerCommand(ctx context.Context, target string, useSudo bool, args ...string) ([]byte, error) {
	if target == "" {
		return nil, fmt.Errorf("container target required")
	}
	cmdArgs := append([]string{"exec", target}, args...)
	if useSudo {
		cmdArgs = append([]string{"docker"}, cmdArgs...)
		cmd := exec.CommandContext(ctx, "sudo", append([]string{"-n"}, cmdArgs...)...)
		return cmd.Output()
	}
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	return cmd.Output()
}

func runFRRShowJSON(ctx context.Context, n ContainerInfo, useSudo bool, command string) (map[string]any, error) {
	target := n.ContainerID
	if target == "" {
		return nil, fmt.Errorf("missing container id for %s", n.Name)
	}
	out, err := runContainerCommand(ctx, target, useSudo, "vtysh", "-c", command)
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func CEOSNodesFromInspect(out []byte) (nodes []ContainerInfo, err error) {
	var parsed InspectResult
	if err = json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	for _, list := range parsed {
		for _, n := range list {
			if strings.EqualFold(n.Kind, "ceos") && n.IPv4 != "" {
				nodes = append(nodes, n)
			}
		}
		break
	}
	return
}

func healthNodesFromInspect(out []byte) (nodes []ContainerInfo, err error) {
	var parsed InspectResult
	if err = json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	for _, list := range parsed {
		for _, n := range list {
			switch {
			case strings.EqualFold(n.Kind, "ceos") && n.IPv4 != "":
				nodes = append(nodes, n)
			case isFRRNode(n):
				nodes = append(nodes, n)
			}
		}
		break
	}
	return
}

func isFRRNode(n ContainerInfo) bool {
	return strings.EqualFold(n.Kind, "linux") && strings.Contains(strings.ToLower(n.Image), "frr")
}

func CIDRIP(s string) string {
	if i := strings.IndexByte(s, '/'); i > 0 {
		return s[:i]
	}
	return s
}

func OnlyNonEmpty(ss []string) (out []string) {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

type frrPeerSummary struct {
	Peer        string
	State       string
	Established bool
	PfxRcd      int
	PfxSnt      int
}

func extractFRRPeerSummaries(data map[string]any) []frrPeerSummary {
	peers, ok := findNestedMap(data, "peers")
	if !ok {
		return nil
	}
	out := make([]frrPeerSummary, 0, len(peers))
	for peer, raw := range peers {
		pm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		state := stringValue(pm, "state", "peerState", "bgpState")
		pfxRcd := intValue(pm, "pfxRcd", "prefixReceivedCount", "receivedPrefixCounter")
		pfxSnt := intValue(pm, "pfxSnt", "prefixSentCount", "sentPrefixCounter")
		established := strings.EqualFold(state, "Established") || strings.EqualFold(state, "established")
		if !established {
			if _, ok := pm["pfxRcd"]; ok {
				established = true
			}
		}
		out = append(out, frrPeerSummary{
			Peer:        peer,
			State:       state,
			Established: established,
			PfxRcd:      pfxRcd,
			PfxSnt:      pfxSnt,
		})
	}
	return out
}

func findNestedMap(data map[string]any, key string) (map[string]any, bool) {
	if v, ok := data[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m, true
		}
	}
	for _, v := range data {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if found, ok := findNestedMap(m, key); ok {
			return found, true
		}
	}
	return nil, false
}

func stringValue(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func intValue(m map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch vv := v.(type) {
			case float64:
				return int(vv)
			case int:
				return vv
			case json.Number:
				if n, err := vv.Int64(); err == nil {
					return int(n)
				}
			case string:
				if n, err := strconv.Atoi(vv); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func countNLRI(data map[string]any) int {
	for _, key := range []string{"advertisedRoutes", "receivedRoutes", "routes"} {
		if routes, ok := findNestedMap(data, key); ok {
			return len(routes)
		}
	}
	return 0
}

func buildFRRAddressFamilyChecks(ctx context.Context, n ContainerInfo, useSudo bool, afiLabel, summaryCmd, advertisedCmd string) []HealthCheck {
	summary, err := runFRRShowJSON(ctx, n, useSudo, summaryCmd)
	if err != nil {
		return []HealthCheck{{
			Name:   afiLabel + " neighbors",
			Result: "FAIL",
			Detail: "summary failed: " + err.Error(),
		}}
	}

	peers := extractFRRPeerSummaries(summary)
	if len(peers) == 0 {
		return []HealthCheck{{
			Name:   afiLabel + " neighbors",
			Result: "WARN",
			Detail: "no peers found in summary output",
		}}
	}

	established := 0
	receivedPeers := 0
	sentPeers := 0
	for _, peer := range peers {
		if peer.Established {
			established++
		}
		if peer.PfxRcd > 0 {
			receivedPeers++
		}
		if peer.PfxSnt > 0 {
			sentPeers++
		}
	}

	checks := []HealthCheck{{
		Name:   afiLabel + " neighbors",
		Result: map[bool]string{true: "PASS", false: "FAIL"}[established == len(peers)],
		Detail: fmt.Sprintf("%d/%d established", established, len(peers)),
	}}

	resultReceived := "WARN"
	if receivedPeers > 0 {
		resultReceived = "PASS"
	}
	checks = append(checks, HealthCheck{
		Name:   afiLabel + " NLRI received",
		Result: resultReceived,
		Detail: fmt.Sprintf("%d/%d peers report received prefixes", receivedPeers, len(peers)),
	})

	if sentPeers > 0 {
		checks = append(checks, HealthCheck{
			Name:   afiLabel + " NLRI advertised",
			Result: "PASS",
			Detail: fmt.Sprintf("%d/%d peers report sent prefixes in summary", sentPeers, len(peers)),
		})
		return checks
	}

	advertisedPeers := 0
	for _, peer := range peers {
		peerCmd := strings.ReplaceAll(advertisedCmd, "{peer}", peer.Peer)
		peerRoutes, err := runFRRShowJSON(ctx, n, useSudo, peerCmd)
		if err != nil {
			continue
		}
		if countNLRI(peerRoutes) > 0 {
			advertisedPeers++
		}
	}
	resultAdvertised := "WARN"
	if advertisedPeers > 0 {
		resultAdvertised = "PASS"
	}
	checks = append(checks, HealthCheck{
		Name:   afiLabel + " NLRI advertised",
		Result: resultAdvertised,
		Detail: fmt.Sprintf("%d/%d peers have advertised prefixes", advertisedPeers, len(peers)),
	})
	return checks
}

func checkFRRNodeHealth(ctx context.Context, n ContainerInfo, useSudo bool) NodeHealth {
	hh := NodeHealth{Name: n.Name, IP: CIDRIP(n.IPv4)}
	hh.Checks = append(hh.Checks,
		buildFRRAddressFamilyChecks(
			ctx,
			n,
			useSudo,
			"FRR IPv4 Unicast",
			"show bgp ipv4 unicast summary json",
			"show bgp ipv4 unicast neighbor {peer} advertised-routes json",
		)...,
	)
	hh.Checks = append(hh.Checks,
		buildFRRAddressFamilyChecks(
			ctx,
			n,
			useSudo,
			"FRR L2VPN EVPN",
			"show bgp l2vpn evpn summary json",
			"show bgp l2vpn evpn neighbor {peer} advertised-routes json",
		)...,
	)
	return hh
}

func checkCEOSNodeHealth(ctx context.Context, n ContainerInfo, user, pass string) NodeHealth {
	checkCmds := []string{
		"show bgp evpn summary",
		"show vxlan vtep",
		"show bgp evpn route-type mac-ip",
	}

	ip := CIDRIP(n.IPv4)
	status, body, err := eapiRun(ctx, ip, user, pass, checkCmds, "json")
	hh := NodeHealth{Name: n.Name, IP: ip}

	if err != nil || status < 200 || status >= 300 {
		hh.Checks = append(hh.Checks, HealthCheck{
			Name:   "eAPI reachability",
			Result: "FAIL",
			Detail: fmt.Sprintf("status=%d err=%v", status, err),
		})
		return hh
	}

	type rpcErr struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	type rpc struct {
		Result []any   `json:"result"`
		Error  *rpcErr `json:"error"`
	}
	var rp rpc
	if err := json.Unmarshal(body, &rp); err != nil {
		hh.Checks = append(hh.Checks, HealthCheck{Name: "parse", Result: "WARN", Detail: "bad JSON from eAPI"})
		return hh
	}
	if rp.Error != nil {
		hh.Checks = append(hh.Checks, HealthCheck{Name: "eAPI error", Result: "FAIL", Detail: rp.Error.Message})
		return hh
	}
	if len(rp.Result) == 0 {
		hh.Checks = append(hh.Checks, HealthCheck{Name: "parse", Result: "WARN", Detail: "empty eAPI result"})
		return hh
	}

	pass1 := false
	if len(rp.Result) >= 1 {
		if r0, ok := rp.Result[0].(map[string]any); ok {
			if vrfs, ok := r0["vrfs"].(map[string]any); ok {
				for _, v := range vrfs {
					if m, ok := v.(map[string]any); ok {
						if peers, ok := m["peers"].(map[string]any); ok {
							for _, p := range peers {
								if pm, ok := p.(map[string]any); ok {
									if st, _ := pm["peerState"].(string); st == "Established" {
										pass1 = true
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}
	hh.Checks = append(hh.Checks, HealthCheck{
		Name:   "EVPN neighbors",
		Result: map[bool]string{true: "PASS", false: "FAIL"}[pass1],
		Detail: map[bool]string{true: "at least one Established", false: "none Established"}[pass1],
	})

	pass2 := false
	if len(rp.Result) >= 2 {
		if r1, ok := rp.Result[1].(map[string]any); ok {
			if m, ok := r1["vteps"].([]any); ok && len(m) > 0 {
				pass2 = true
			}
		}
	}
	hh.Checks = append(hh.Checks, HealthCheck{
		Name:   "VXLAN VTEPs",
		Result: map[bool]string{true: "PASS", false: "WARN"}[pass2],
		Detail: map[bool]string{true: "remote VTEPs present", false: "no remote VTEPs"}[pass2],
	})

	pass3 := false
	if len(rp.Result) >= 3 {
		if r2, ok := rp.Result[2].(map[string]any); ok {
			if routes, ok := r2["routes"].(map[string]any); ok && len(routes) > 0 {
				pass3 = true
			}
		}
	}
	hh.Checks = append(hh.Checks, HealthCheck{
		Name:   "EVPN MAC/IP routes",
		Result: map[bool]string{true: "PASS", false: "WARN"}[pass3],
		Detail: map[bool]string{true: "MAC/IP routes present", false: "no MAC/IP routes"}[pass3],
	})
	return hh
}

func eapiRun(ctx context.Context, ip, user, pass string, cmds []string, format string) (status int, body []byte, err error) {
	if format == "" {
		format = "json"
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "runCmds",
		"params": map[string]any{
			"version": 1,
			"format":  format,
			"cmds":    cmds,
		},
		"id": 1,
	}
	b, _ := json.Marshal(payload)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	url := "https://" + ip + "/command-api"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

func (h *Handlers) Labs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var (
		labs []labstore.LabRecord
		err  error
	)
	db, dbErr := labstore.OpenLabDB(h.cfg.BaseDir)
	if dbErr == nil {
		defer db.Close()
		labs, err = labstore.ListLabs(db)
		if err != nil {
			labs = nil
		}
	}
	labs, err = mergeLabsWithFilesystem(h.cfg.BaseDir, labs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LabsResponse{OK: false, Error: err.Error()})
		return
	}
	for i := range labs {
		labs[i].NodeType = detectLabNodeType(labs[i].Path)
	}
	writeJSON(w, http.StatusOK, LabsResponse{OK: true, Labs: labs})
}

func detectLabNodeType(labPath string) string {
	body, err := os.ReadFile(filepath.Join(labPath, "lab.clab.yml"))
	if err != nil {
		return "unknown"
	}
	text := strings.ToLower(string(body))
	switch {
	case strings.Contains(text, "quay.io/frrouting/frr") || strings.Contains(text, "/etc/frr/"):
		return "frr"
	case strings.Contains(text, "ceosimage:") || strings.Contains(text, "kind: ceos"):
		return "arista"
	default:
		return "unknown"
	}
}

func (h *Handlers) LabPlan(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, LabPlanResponse{OK: false, Error: "name is required"})
		return
	}
	db, err := labstore.OpenLabDB(h.cfg.BaseDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LabPlanResponse{OK: false, Error: err.Error()})
		return
	}
	defer db.Close()

	plan, err := labstore.LoadLabPlan(db, name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LabPlanResponse{OK: false, Error: err.Error()})
		return
	}

	var nodes []NodePlanJSON
	for _, n := range plan.Nodes {
		nodes = append(nodes, NodePlanJSON{
			Name:       n.Name,
			Role:       n.Role,
			ASN:        n.ASN,
			Loopback:   n.Loopback,
			EdgeIP:     n.EdgeIP,
			EdgePrefix: n.EdgePrefix,
		})
	}

	var links []LinkAssignedJSON
	for _, l := range plan.Links {
		links = append(links, LinkAssignedJSON{
			A:   l.A,
			B:   l.B,
			AIf: l.AIf,
			BIf: l.BIf,
		})
	}

	writeJSON(w, http.StatusOK, LabPlanResponse{
		OK:    true,
		Nodes: nodes,
		Links: links,
		Protocols: ProtocolSetJSON{
			Global: plan.Protocols.Global,
			Roles:  plan.Protocols.Roles,
		},
	})
}

func (h *Handlers) TopologyLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LiveTopologyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LiveTopologyResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	labName := strings.TrimSpace(req.LabName)
	if labName == "" {
		writeJSON(w, http.StatusBadRequest, LiveTopologyResponse{OK: false, Error: "labName is required"})
		return
	}
	if !isSafeName(labName) {
		writeJSON(w, http.StatusBadRequest, LiveTopologyResponse{OK: false, Error: "labName must be alphanumeric, dash, or underscore"})
		return
	}
	labPath := filepath.Join(h.cfg.BaseDir, labName, "lab.clab.yml")
	if _, err := os.Stat(labPath); err != nil {
		writeJSON(w, http.StatusBadRequest, LiveTopologyResponse{OK: false, Error: "lab file not found at " + labPath})
		return
	}

	db, err := labstore.OpenLabDB(h.cfg.BaseDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LiveTopologyResponse{OK: false, Error: err.Error()})
		return
	}
	defer db.Close()
	plan, err := labstore.LoadLabPlan(db, labName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LiveTopologyResponse{OK: false, Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := runInspect(ctx, labPath, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LiveTopologyResponse{OK: false, Error: "inspect failed: " + err.Error()})
		return
	}
	containers, err := inspectNodesFromOutput(out)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LiveTopologyResponse{OK: false, Error: "parse inspect: " + err.Error()})
		return
	}
	nodeContainer := map[string]ContainerInfo{}
	for _, c := range containers {
		name := shortNodeName(c.Name, labName)
		if name == "" {
			continue
		}
		nodeContainer[name] = c
	}

	var nodes []NodePlanJSON
	for _, n := range plan.Nodes {
		nodes = append(nodes, NodePlanJSON{
			Name:       n.Name,
			Role:       n.Role,
			ASN:        n.ASN,
			Loopback:   n.Loopback,
			EdgeIP:     n.EdgeIP,
			EdgePrefix: n.EdgePrefix,
		})
	}

	ifaceStateCache := map[string]string{}
	linkStatuses := make([]LiveLinkStatus, 0, len(plan.Links))
	summary := LiveSummary{Total: len(plan.Links)}
	for _, link := range plan.Links {
		aState := endpointOperState(ctx, nodeContainer, ifaceStateCache, link.A, link.AIf, req.UseSudo)
		bState := endpointOperState(ctx, nodeContainer, ifaceStateCache, link.B, link.BIf, req.UseSudo)
		state := combineLinkState(aState, bState)
		switch state {
		case "up":
			summary.Up++
		case "down":
			summary.Down++
		default:
			summary.Unknown++
		}
		linkStatuses = append(linkStatuses, LiveLinkStatus{
			A:     link.A,
			B:     link.B,
			AIf:   link.AIf,
			BIf:   link.BIf,
			State: state,
			Endpoints: []LiveEndpointStatus{
				{Node: link.A, Iface: link.AIf, State: aState},
				{Node: link.B, Iface: link.BIf, State: bState},
			},
		})
	}

	writeJSON(w, http.StatusOK, LiveTopologyResponse{
		OK:       true,
		LabName:  labName,
		Nodes:    nodes,
		Links:    linkStatuses,
		Summary:  summary,
		PolledAt: time.Now().Format(time.RFC3339),
	})
}

func inspectNodesFromOutput(out []byte) ([]ContainerInfo, error) {
	var parsed InspectResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	for _, list := range parsed {
		return list, nil
	}
	return nil, nil
}

func shortNodeName(containerName, labName string) string {
	name := strings.TrimSpace(containerName)
	if name == "" {
		return ""
	}
	prefix := "clab-" + labName + "-"
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	return name
}

func endpointOperState(ctx context.Context, nodeMap map[string]ContainerInfo, cache map[string]string, nodeName, ifName string, useSudo bool) string {
	if strings.TrimSpace(nodeName) == "" || strings.TrimSpace(ifName) == "" {
		return "unknown"
	}
	key := nodeName + "|" + ifName
	if v, ok := cache[key]; ok {
		return v
	}
	n, ok := nodeMap[nodeName]
	if !ok {
		cache[key] = "unknown"
		return "unknown"
	}
	target := n.ContainerID
	if strings.TrimSpace(target) == "" {
		target = n.Name
	}
	if strings.TrimSpace(target) == "" {
		cache[key] = "unknown"
		return "unknown"
	}
	cmd := fmt.Sprintf("cat /sys/class/net/%s/operstate 2>/dev/null || echo unknown", ifName)
	out, err := runContainerCommand(ctx, target, useSudo, "sh", "-lc", cmd)
	if err != nil {
		cache[key] = "unknown"
		return "unknown"
	}
	state := strings.ToLower(strings.TrimSpace(string(out)))
	switch state {
	case "up":
		cache[key] = "up"
	case "down":
		cache[key] = "down"
	default:
		cache[key] = "unknown"
	}
	return cache[key]
}

func combineLinkState(aState, bState string) string {
	if aState == "up" && bState == "up" {
		return "up"
	}
	if aState == "down" || bState == "down" {
		return "down"
	}
	return "unknown"
}

func (h *Handlers) LabNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LabNodesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LabNodesResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}

	labAbs, err := h.cfg.SanitizeLabPath(req.Lab)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LabNodesResponse{OK: false, Error: err.Error()})
		return
	}

	nodes, err := listConfigNodes(filepath.Join(filepath.Dir(labAbs), "configs"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LabNodesResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LabNodesResponse{OK: true, Nodes: nodes})
}

func (h *Handlers) LabNodeConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LabNodeConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, LabNodeConfigResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	req.NodeName = strings.TrimSpace(req.NodeName)
	if req.NodeName == "" {
		writeJSON(w, http.StatusBadRequest, LabNodeConfigResponse{OK: false, Error: "node name is required"})
		return
	}

	labAbs, err := h.cfg.SanitizeLabPath(req.Lab)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LabNodeConfigResponse{OK: false, Error: err.Error()})
		return
	}

	root := filepath.Dir(labAbs)
	cfgPath := filepath.Join(root, "configs", req.NodeName+".cfg")
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, LabNodeConfigResponse{OK: false, Error: "node config not found"})
		return
	}

	resp := LabNodeConfigResponse{
		OK:       true,
		NodeName: req.NodeName,
		Config:   string(body),
	}
	daemonsPath := filepath.Join(root, "configs", req.NodeName+".daemons")
	if daemons, err := os.ReadFile(daemonsPath); err == nil {
		resp.Daemons = string(daemons)
	}
	startup, _ := nodeStartupConfig(labAbs, req.NodeName)
	resp.Startup = startup
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) Inspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req inspectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, inspectResp{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}

	labAbs, err := h.cfg.SanitizeLabPath(req.Lab)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, inspectResp{OK: false, Error: err.Error()})
		return
	}

	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 60*time.Second {
		timeout = 15 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	out, err := runInspect(ctx, labAbs, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, inspectResp{OK: false, Error: "inspect failed: " + err.Error()})
		return
	}

	var parsed InspectResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		writeJSON(w, http.StatusInternalServerError, inspectResp{OK: false, Error: "parse failed: " + err.Error()})
		return
	}

	var key string
	var nodes []ContainerInfo
	for k, v := range parsed {
		key, nodes = k, v
		break
	}

	writeJSON(w, http.StatusOK, inspectResp{
		OK:      true,
		LabKey:  key,
		Nodes:   nodes,
		RawJSON: out,
	})
}

func listConfigNodes(cfgDir string) ([]string, error) {
	entries, err := os.ReadDir(cfgDir)
	if err != nil {
		return nil, fmt.Errorf("configs directory not found")
	}
	var nodes []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".cfg") {
			nodes = append(nodes, strings.TrimSuffix(name, ".cfg"))
		}
	}
	sort.Strings(nodes)
	return nodes, nil
}

func mergeLabsWithFilesystem(baseDir string, indexed []labstore.LabRecord) ([]labstore.LabRecord, error) {
	merged := map[string]labstore.LabRecord{}
	for _, lab := range indexed {
		merged[lab.Name] = lab
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return indexed, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		labPath := filepath.Join(baseDir, entry.Name(), "lab.clab.yml")
		info, err := os.Stat(labPath)
		if err != nil || info.IsDir() {
			continue
		}
		if _, ok := merged[entry.Name()]; ok {
			continue
		}
		merged[entry.Name()] = labstore.LabRecord{
			Name:      entry.Name(),
			Path:      filepath.Join(baseDir, entry.Name()),
			CreatedAt: info.ModTime(),
		}
	}

	out := make([]labstore.LabRecord, 0, len(merged))
	for _, lab := range merged {
		out = append(out, lab)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Name < out[j].Name
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func nodeStartupConfig(labPath, nodeName string) (string, error) {
	body, err := os.ReadFile(labPath)
	if err == nil {
		if startup := extractNodeExec(string(body), nodeName); startup != "" {
			return startup, nil
		}
	}

	scriptPath := filepath.Join(filepath.Dir(labPath), "configs", nodeName+".sh")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", err
	}
	return string(script), nil
}

func extractNodeExec(yamlBody, nodeName string) string {
	lines := strings.Split(yamlBody, "\n")
	inNode := false
	inExec := false
	var execLines []string
	nodeHeader := "    " + nodeName + ":"

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "    ") && strings.TrimSpace(line) == strings.TrimSpace(nodeHeader):
			inNode = true
			inExec = false
		case inNode && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      "):
			inNode = false
			inExec = false
		case inNode && strings.TrimSpace(line) == "exec:":
			inExec = true
		case inNode && inExec && strings.HasPrefix(line, "        - "):
			execLines = append(execLines, strings.TrimPrefix(line, "        - "))
		case inNode && inExec && strings.HasPrefix(line, "      ") && !strings.HasPrefix(line, "        - "):
			inExec = false
		}
	}

	return strings.Join(execLines, "\n")
}

func (h *Handlers) RunCmd(w http.ResponseWriter, r *http.Request) {
	cmds := []string{"show bgp summary"}
	url := "https://172.20.20.9/command-api"
	client := arista.NewEosClient(url)
	tmplPath := "templates/eapi_payload.tmpl"
	body, err := renderer.RenderTemplate(tmplPath, renderer.PayloadData{
		Method:  "runCmds",
		Version: 1,
		Format:  "json",
		Cmds:    cmds,
	})
	if err != nil {
		http.Error(w, "template render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var resp map[string]any
	if err := client.Run(body, &resp); err != nil {
		http.Error(w, "command failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req HealthReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, HealthResp{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	labAbs, err := h.cfg.SanitizeLabPath(req.Lab)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, HealthResp{OK: false, Error: err.Error()})
		return
	}

	tout := time.Duration(req.TimeoutSec) * time.Second
	if tout <= 0 || tout > 60*time.Second {
		tout = 20 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), tout)
	defer cancel()
	out, err := runInspect(ctx, labAbs, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, HealthResp{OK: false, Error: "inspect failed: " + err.Error()})
		return
	}
	nodes, err := healthNodesFromInspect(out)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, HealthResp{OK: false, Error: "parse inspect: " + err.Error()})
		return
	}
	if len(nodes) == 0 {
		writeJSON(w, http.StatusOK, HealthResp{OK: true, Nodes: nil})
		return
	}

	type item struct {
		i  int
		nh NodeHealth
	}
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	ch := make(chan item, len(nodes))

	for i, n := range nodes {
		wg.Add(1)
		go func(i int, n ContainerInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cx, cancel := context.WithTimeout(r.Context(), tout)
			defer cancel()

			var hh NodeHealth
			if isFRRNode(n) {
				hh = checkFRRNodeHealth(cx, n, req.UseSudo)
			} else {
				hh = checkCEOSNodeHealth(cx, n, req.User, req.Pass)
			}
			ch <- item{i: i, nh: hh}
		}(i, n)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	results := make([]NodeHealth, len(nodes))
	for it := range ch {
		results[it.i] = it.nh
	}
	writeJSON(w, http.StatusOK, HealthResp{OK: true, Nodes: results})
}
