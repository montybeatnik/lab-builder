package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/montybeatnik/arista-lab/laber/configgenerator"
	"github.com/montybeatnik/arista-lab/laber/labplanner"
	"github.com/montybeatnik/arista-lab/laber/labstore"
)

type TopologyRequest struct {
	Topology    string      `json:"topology"`
	NodeCount   int         `json:"nodeCount"`
	LeafCount   int         `json:"leafCount"`
	SpineCount  int         `json:"spineCount"`
	HubCount    int         `json:"hubCount"`
	SpokeCount  int         `json:"spokeCount"`
	EdgeNodes   int         `json:"edgeNodes"`
	InfraCIDR   string      `json:"infraCidr"`
	EdgeCIDR    string      `json:"edgeCidr"`
	CustomLinks []LinkInput `json:"customLinks"`
	EdgeLinks   []EdgeLinkInput `json:"edgeLinks"`
	Traffic     []Traffic   `json:"traffic"`
	LabName     string      `json:"labName"`
	Force       bool        `json:"force"`
	UseSudo     bool        `json:"sudo"`
	Protocols   labplanner.ProtocolSet `json:"protocols"`
}

type LinkInput struct {
	A string `json:"a"`
	B string `json:"b"`
}

type EdgeLinkInput struct {
	Edge   string `json:"edge"`
	Target string `json:"target"`
}

type Traffic struct {
	Profile string `json:"profile"`
	Level   int    `json:"level"`
}

type Check struct {
	Name   string `json:"name"`
	Result string `json:"result"` // PASS | WARN | FAIL
	Detail string `json:"detail,omitempty"`
}

type AddressSummary struct {
	InfraCIDR   string `json:"infraCidr"`
	EdgeCIDR    string `json:"edgeCidr"`
	InfraTotal  int64  `json:"infraTotal"`
	InfraNeeded int64  `json:"infraNeeded"`
	EdgeTotal   int64  `json:"edgeTotal"`
	EdgeNeeded  int64  `json:"edgeNeeded"`
	Loopbacks   int64  `json:"loopbacks"`
	P2PLinks    int64  `json:"p2pLinks"`
}

type TopologyResponse struct {
	OK        bool           `json:"ok"`
	Errors    []string       `json:"errors,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
	Checks    []Check        `json:"checks,omitempty"`
	Model     labplanner.TopologyModel `json:"model,omitempty"`
	Address   AddressSummary `json:"address,omitempty"`
	CanBuild  bool           `json:"canBuild"`
	Notes     []string       `json:"notes,omitempty"`
	RawTarget json.RawMessage `json:"rawTarget,omitempty"`
}

type BuildResponse struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Path    string   `json:"path,omitempty"`
	Files   []string `json:"files,omitempty"`
}

type DeployRequest struct {
	LabName string `json:"labName"`
	UseSudo bool   `json:"sudo"`
	Force   bool   `json:"force"`
}

type DeployResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Output string `json:"output,omitempty"`
	Path   string `json:"path,omitempty"`
}

func topologyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req TopologyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, TopologyResponse{
			OK:     false,
			Errors: []string{"bad JSON: " + err.Error()},
		})
		return
	}

	model, errs, warns := buildTopologyModel(req)
	addr, addrChecks, addrErrs, addrWarns := validateAddressing(req, model)
	errs = append(errs, addrErrs...)
	warns = append(warns, addrWarns...)

	checks := append([]Check{}, addrChecks...)
	checks = append(checks, topologyChecks(model, errs, warns)...)

	ok := len(errs) == 0
	writeJSON(w, http.StatusOK, TopologyResponse{
		OK:       ok,
		Errors:   errs,
		Warnings: warns,
		Checks:   checks,
		Model:    model,
		Address:  addr,
		CanBuild: ok,
	})
}

func topologyBuildHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req TopologyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: "bad JSON: " + err.Error()})
			return
		}
		labName := strings.TrimSpace(req.LabName)
		if labName == "" {
			writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: "lab name is required"})
			return
		}
		if !isSafeName(labName) {
			writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: "lab name must be alphanumeric, dash, or underscore"})
			return
		}

		model, errs, warns := buildTopologyModel(req)
		_, _, addrErrs, addrWarns := validateAddressing(req, model)
		errs = append(errs, addrErrs...)
		warns = append(warns, addrWarns...)
		if len(errs) > 0 {
			writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: strings.Join(errs, "; "), Warnings: warns})
			return
		}

		root := filepath.Join(cfg.BaseDir, labName)
		if !req.Force {
			if _, err := os.Stat(root); err == nil {
				writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: "lab directory already exists"})
				return
			}
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, BuildResponse{OK: false, Error: "mkdir failed: " + err.Error()})
			return
		}

		plan, err := labplanner.BuildLabPlan(req.InfraCIDR, req.EdgeCIDR, model, toEdgeAttachments(req.EdgeLinks))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: err.Error()})
			return
		}

		files, err := writeLabFiles(root, labName, model, plan)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, BuildResponse{OK: false, Error: err.Error()})
			return
		}

		if db, err := labstore.OpenLabDB(cfg.BaseDir); err == nil {
			_ = labstore.UpsertLab(db, labName, root)
			_ = labstore.SaveLabPlan(db, labName, plan, model.Protocols)
			_ = db.Close()
		}
		writeJSON(w, http.StatusOK, BuildResponse{OK: true, Path: root, Files: files, Warnings: warns})
	}
}

func topologyDeployHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req DeployRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: "bad JSON: " + err.Error()})
			return
		}
		labName := strings.TrimSpace(req.LabName)
		if labName == "" {
			writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: "lab name is required"})
			return
		}
		if !isSafeName(labName) {
			writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: "lab name must be alphanumeric, dash, or underscore"})
			return
		}

		labPath := filepath.Join(cfg.BaseDir, labName, "lab.clab.yml")
		if _, err := os.Stat(labPath); err != nil {
			writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: "lab file not found at " + labPath})
			return
		}

		args := []string{"containerlab", "deploy", "-t", labPath, "--reconfigure"}
		if req.UseSudo {
			args = append([]string{"sudo", "-E", "-n"}, args...)
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		clabBase := "/home/ubuntu/.clab-runs"
		_ = os.MkdirAll(clabBase, 0o755)
		cmd.Env = append(os.Environ(), "CLAB_LABDIR_BASE="+clabBase)
		output, err := cmd.CombinedOutput()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: err.Error(), Output: string(output), Path: labPath})
			return
		}

		writeJSON(w, http.StatusOK, DeployResponse{OK: true, Output: string(output), Path: labPath})
	}
}

func writeLabFiles(root, labName string, model labplanner.TopologyModel, plan labplanner.LabPlan) ([]string, error) {
	var files []string
	yamlBody := configgenerator.RenderContainerlabYAML(labName, model, plan.Links, plan.EdgeHosts)
	yamlPath := filepath.Join(root, "lab.clab.yml")
	if err := os.WriteFile(yamlPath, []byte(yamlBody), 0o644); err != nil {
		return files, err
	}
	files = append(files, yamlPath)

	cfgDir := filepath.Join(root, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return files, err
	}
	nodeLinks := map[string][]labplanner.LinkAssigned{}
	for _, link := range plan.Links {
		nodeLinks[link.A] = append(nodeLinks[link.A], link)
		nodeLinks[link.B] = append(nodeLinks[link.B], link)
	}

	nodeMap := map[string]labplanner.NodePlan{}
	for _, node := range plan.Nodes {
		nodeMap[node.Name] = node
	}

	for _, node := range plan.Nodes {
		if node.Role == "edge" {
			continue
		}
		path := filepath.Join(cfgDir, node.Name+".cfg")
		body, err := configgenerator.RenderNodeConfig("templates/config/node.tmpl", node, nodeLinks[node.Name], nodeMap)
		if err != nil {
			return files, err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return files, err
		}
		files = append(files, path)
	}

	trafficDir := filepath.Join(root, "traffic")
	if err := os.MkdirAll(trafficDir, 0o755); err != nil {
		return files, err
	}
	serverDockerfile := filepath.Join(trafficDir, "Dockerfile.server")
	serverBody := "FROM golang:1.23-alpine AS build\n" +
		"WORKDIR /src\n" +
		"COPY . .\n" +
		"RUN CGO_ENABLED=0 go build -o /out/traffic-server ./cmd/traffic-server\n\n" +
		"FROM alpine:3.19\n" +
		"COPY --from=build /out/traffic-server /traffic/traffic-server\n" +
		"EXPOSE 8081 5004\n" +
		"ENTRYPOINT [\"/traffic/traffic-server\"]\n"
	if err := os.WriteFile(serverDockerfile, []byte(serverBody), 0o644); err != nil {
		return files, err
	}
	files = append(files, serverDockerfile)

	clientDockerfile := filepath.Join(trafficDir, "Dockerfile.client")
	clientBody := "FROM golang:1.23-alpine AS build\n" +
		"WORKDIR /src\n" +
		"COPY . .\n" +
		"RUN CGO_ENABLED=0 go build -o /out/traffic-client ./cmd/traffic-client\n\n" +
		"FROM alpine:3.19\n" +
		"COPY --from=build /out/traffic-client /traffic/traffic-client\n" +
		"ENTRYPOINT [\"/traffic/traffic-client\"]\n"
	if err := os.WriteFile(clientDockerfile, []byte(clientBody), 0o644); err != nil {
		return files, err
	}
	files = append(files, clientDockerfile)

	readme := filepath.Join(trafficDir, "README.txt")
	readmeBody := "Build traffic images from the repo's src/traffic:\n" +
		"  docker build -t lab-traffic-server -f src/traffic/Dockerfile.server src/traffic\n" +
		"  docker build -t lab-traffic-client -f src/traffic/Dockerfile.client src/traffic\n"
	if err := os.WriteFile(readme, []byte(readmeBody), 0o644); err != nil {
		return files, err
	}
	files = append(files, readme)

	return files, nil
}

func isSafeName(name string) bool {
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func toEdgeAttachments(links []EdgeLinkInput) []labplanner.EdgeAttachment {
	var out []labplanner.EdgeAttachment
	for _, l := range links {
		edge := strings.TrimSpace(l.Edge)
		target := strings.TrimSpace(l.Target)
		if edge == "" || target == "" {
			continue
		}
		out = append(out, labplanner.EdgeAttachment{Edge: edge, Target: target})
	}
	return out
}


func buildTopologyModel(req TopologyRequest) (labplanner.TopologyModel, []string, []string) {
	var errs []string
	var warns []string
	topology := strings.ToLower(strings.TrimSpace(req.Topology))
	if topology == "" {
		topology = "leaf-spine"
		warns = append(warns, "topology not set; defaulting to leaf-spine")
	}
	model := labplanner.TopologyModel{
		Topology:  topology,
		EdgeNodes: maxInt(req.EdgeNodes, 0),
		Protocols: labplanner.NormalizeProtocols(req.Protocols),
	}

	switch topology {
	case "leaf-spine":
		spines := clamp(req.SpineCount, 1, 64)
		leaves := clamp(req.LeafCount, 1, 256)
		if req.SpineCount <= 0 {
			errs = append(errs, "spine count must be > 0")
		}
		if req.LeafCount <= 0 {
			errs = append(errs, "leaf count must be > 0")
		}
		for i := 1; i <= spines; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "spine" + itoa(i), Role: "spine"})
		}
		for i := 1; i <= leaves; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "leaf" + itoa(i), Role: "leaf"})
		}
		for i := 1; i <= spines; i++ {
			for j := 1; j <= leaves; j++ {
				model.Links = append(model.Links, labplanner.TopologyLink{
					A: "spine" + itoa(i),
					B: "leaf" + itoa(j),
				})
			}
		}
	case "full-mesh":
		n := clamp(req.NodeCount, 2, 256)
		if req.NodeCount < 2 {
			errs = append(errs, "node count must be >= 2")
		}
		for i := 1; i <= n; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "node" + itoa(i), Role: "mesh"})
		}
		for i := 1; i <= n; i++ {
			for j := i + 1; j <= n; j++ {
				model.Links = append(model.Links, labplanner.TopologyLink{
					A: "node" + itoa(i),
					B: "node" + itoa(j),
				})
			}
		}
	case "hub-spoke":
		hubs := clamp(req.HubCount, 1, 16)
		spokes := clamp(req.SpokeCount, 1, 256)
		if req.HubCount <= 0 {
			errs = append(errs, "hub count must be > 0")
		}
		if req.SpokeCount <= 0 {
			errs = append(errs, "spoke count must be > 0")
		}
		for i := 1; i <= hubs; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "hub" + itoa(i), Role: "hub"})
		}
		for i := 1; i <= spokes; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "spoke" + itoa(i), Role: "spoke"})
		}
		for i := 1; i <= hubs; i++ {
			for j := 1; j <= spokes; j++ {
				model.Links = append(model.Links, labplanner.TopologyLink{
					A: "hub" + itoa(i),
					B: "spoke" + itoa(j),
				})
			}
		}
	case "custom":
		n := clamp(req.NodeCount, 1, 256)
		if req.NodeCount < 1 {
			errs = append(errs, "node count must be >= 1")
		}
		for i := 1; i <= n; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "node" + itoa(i), Role: "custom"})
		}
		validNames := map[string]bool{}
		for _, n := range model.Nodes {
			validNames[n.Name] = true
		}
		linkSet := map[string]bool{}
		for _, l := range req.CustomLinks {
			a := strings.TrimSpace(l.A)
			b := strings.TrimSpace(l.B)
			if a == "" || b == "" {
				continue
			}
			if a == b {
				errs = append(errs, "custom links cannot connect a node to itself ("+a+")")
				continue
			}
			if !validNames[a] || !validNames[b] {
				errs = append(errs, "custom link references unknown node ("+a+"-"+b+")")
				continue
			}
			key := linkKey(a, b)
			if linkSet[key] {
				warns = append(warns, "duplicate custom link ignored ("+a+"-"+b+")")
				continue
			}
			linkSet[key] = true
			model.Links = append(model.Links, labplanner.TopologyLink{A: a, B: b})
		}
		if len(req.CustomLinks) == 0 {
			warns = append(warns, "no custom links provided")
		}
	default:
		errs = append(errs, "unknown topology type: "+topology)
	}

	if req.EdgeNodes < 0 {
		errs = append(errs, "edge nodes must be >= 0")
	}

	return model, errs, warns
}

func topologyChecks(model labplanner.TopologyModel, errs, warns []string) []Check {
	result := "PASS"
	detail := "inputs look good"
	if len(errs) > 0 {
		result = "FAIL"
		detail = strings.Join(errs, "; ")
	} else if len(warns) > 0 {
		result = "WARN"
		detail = strings.Join(warns, "; ")
	}
	check := Check{Name: "Topology inputs", Result: result, Detail: detail}
	return []Check{check}
}

func validateAddressing(req TopologyRequest, model labplanner.TopologyModel) (AddressSummary, []Check, []string, []string) {
	var errs []string
	var warns []string
	var checks []Check
	addr := AddressSummary{
		InfraCIDR: strings.TrimSpace(req.InfraCIDR),
		EdgeCIDR:  strings.TrimSpace(req.EdgeCIDR),
	}

	loopbacks := int64(len(model.Nodes))
	p2pLinks := int64(len(model.Links))
	addr.Loopbacks = loopbacks
	addr.P2PLinks = p2pLinks
	addr.InfraNeeded = loopbacks + (p2pLinks * 2)
	addr.EdgeNeeded = int64(maxInt(req.EdgeNodes, 0))

	if addr.InfraCIDR == "" {
		errs = append(errs, "infra CIDR is required")
	} else {
		total, err := ipv4Capacity(addr.InfraCIDR)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			addr.InfraTotal = total
			if addr.InfraTotal < addr.InfraNeeded {
				errs = append(errs, "infra CIDR does not have enough addresses for loopbacks and p2p links")
			}
		}
	}

	if addr.EdgeCIDR == "" {
		warns = append(warns, "edge CIDR not provided; edge nodes cannot be addressed")
	} else {
		total, err := ipv4Capacity(addr.EdgeCIDR)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			addr.EdgeTotal = total
			if addr.EdgeNeeded == 0 {
				warns = append(warns, "edge CIDR provided but edge node count is zero")
			} else if addr.EdgeTotal < addr.EdgeNeeded {
				errs = append(errs, "edge CIDR does not have enough addresses for edge nodes")
			}
		}
	}

	checks = append(checks, Check{
		Name:   "Addressing capacity",
		Result: checkResult(errs, warns),
		Detail: addressDetail(addr),
	})
	return addr, checks, errs, warns
}

func addressDetail(addr AddressSummary) string {
	parts := []string{
		"loopbacks=" + itoa64(addr.Loopbacks),
		"p2pLinks=" + itoa64(addr.P2PLinks),
		"infraNeeded=" + itoa64(addr.InfraNeeded),
	}
	if addr.InfraTotal > 0 {
		parts = append(parts, "infraTotal="+itoa64(addr.InfraTotal))
	}
	if addr.EdgeTotal > 0 {
		parts = append(parts, "edgeTotal="+itoa64(addr.EdgeTotal))
	}
	if addr.EdgeNeeded > 0 {
		parts = append(parts, "edgeNeeded="+itoa64(addr.EdgeNeeded))
	}
	return strings.Join(parts, ", ")
}

func checkResult(errs, warns []string) string {
	if len(errs) > 0 {
		return "FAIL"
	}
	if len(warns) > 0 {
		return "WARN"
	}
	return "PASS"
}

func ipv4Capacity(cidr string) (int64, error) {
	ip, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return 0, err
	}
	if ip == nil || ip.To4() == nil {
		return 0, errInvalidCIDR("only IPv4 CIDRs are supported")
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return 0, errInvalidCIDR("only IPv4 CIDRs are supported")
	}
	hostBits := 32 - ones
	if hostBits < 0 {
		return 0, errInvalidCIDR("invalid CIDR")
	}
	if hostBits > 62 {
		return 0, errInvalidCIDR("CIDR too large")
	}
	return int64(1) << hostBits, nil
}

type cidrError string

func (e cidrError) Error() string { return string(e) }

func errInvalidCIDR(msg string) error {
	return cidrError("CIDR error: " + msg)
}

func linkKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "--" + b
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func itoa64(i int64) string {
	return strconv.FormatInt(i, 10)
}
