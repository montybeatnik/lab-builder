package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// execCommandContext is injected in tests so lifecycle handlers can be covered
// without needing a real containerlab binary on the test host.
var execCommandContext = exec.CommandContext

const requiredAristaImage = "ceosimage:4.34.2.1f"

func (h *Handlers) TopologyAristaImageStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	present, err := aristaImagePresent(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, AristaImageStatusResponse{
			OK:            false,
			Error:         err.Error(),
			Present:       false,
			RequiredImage: requiredAristaImage,
			ArchivePath:   h.aristaImageArchivePath(""),
		})
		return
	}
	writeJSON(w, http.StatusOK, AristaImageStatusResponse{
		OK:            true,
		Present:       present,
		RequiredImage: requiredAristaImage,
		ArchivePath:   h.aristaImageArchivePath(""),
	})
}

func (h *Handlers) TopologyAristaImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, AristaImageUploadResponse{
			OK:            false,
			Error:         "invalid multipart form: " + err.Error(),
			RequiredImage: requiredAristaImage,
		})
		return
	}
	file, hdr, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, AristaImageUploadResponse{
			OK:            false,
			Error:         "image upload is required (form field: image)",
			RequiredImage: requiredAristaImage,
		})
		return
	}
	defer file.Close()

	archivePath, err := h.saveAristaArchive(hdr.Filename, file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, AristaImageUploadResponse{
			OK:            false,
			Error:         err.Error(),
			RequiredImage: requiredAristaImage,
		})
		return
	}

	out, err := dockerImageLoad(r.Context(), archivePath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, AristaImageUploadResponse{
			OK:            false,
			Error:         err.Error(),
			RequiredImage: requiredAristaImage,
			ArchivePath:   archivePath,
			Output:        strings.TrimSpace(string(out)),
		})
		return
	}

	present, checkErr := aristaImagePresent(r.Context())
	if checkErr != nil {
		writeJSON(w, http.StatusBadRequest, AristaImageUploadResponse{
			OK:            false,
			Error:         checkErr.Error(),
			RequiredImage: requiredAristaImage,
			ArchivePath:   archivePath,
			Output:        strings.TrimSpace(string(out)),
		})
		return
	}
	if !present {
		writeJSON(w, http.StatusBadRequest, AristaImageUploadResponse{
			OK:            false,
			Error:         "image loaded, but required tag not present; retag archive to " + requiredAristaImage,
			RequiredImage: requiredAristaImage,
			ArchivePath:   archivePath,
			Output:        strings.TrimSpace(string(out)),
		})
		return
	}

	writeJSON(w, http.StatusOK, AristaImageUploadResponse{
		OK:            true,
		RequiredImage: requiredAristaImage,
		ArchivePath:   archivePath,
		Output:        strings.TrimSpace(string(out)),
	})
}

func (h *Handlers) TopologyValidate(w http.ResponseWriter, r *http.Request) {
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

	model, errs, warns := BuildTopologyModel(req)
	if nodeTypeIsArista(req.NodeType) {
		present, err := aristaImagePresent(r.Context())
		if err != nil {
			errs = append(errs, "could not verify Arista image "+requiredAristaImage+": "+err.Error())
		} else if !present {
			errs = append(errs, "Arista image "+requiredAristaImage+" not found; upload it first or switch node type to FRR")
		}
	}
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

func (h *Handlers) TopologyBuild(w http.ResponseWriter, r *http.Request) {
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

	model, errs, warns := BuildTopologyModel(req)
	if nodeTypeIsArista(req.NodeType) {
		present, err := aristaImagePresent(r.Context())
		if err != nil {
			errs = append(errs, "could not verify Arista image "+requiredAristaImage+": "+err.Error())
		} else if !present {
			errs = append(errs, "Arista image "+requiredAristaImage+" not found; upload it first or switch node type to FRR")
		}
	}
	_, _, addrErrs, addrWarns := validateAddressing(req, model)
	errs = append(errs, addrErrs...)
	warns = append(warns, addrWarns...)
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, BuildResponse{OK: false, Error: strings.Join(errs, "; "), Warnings: warns})
		return
	}

	root := filepath.Join(h.cfg.BaseDir, labName)
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

	files, err := writeLabFiles(root, labName, model, plan, req.Monitoring)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, BuildResponse{OK: false, Error: err.Error()})
		return
	}

	if db, err := labstore.OpenLabDB(h.cfg.BaseDir); err == nil {
		_ = labstore.UpsertLab(db, labName, root)
		_ = labstore.SaveLabPlan(db, labName, plan, model.Protocols)
		_ = db.Close()
	}
	writeJSON(w, http.StatusOK, BuildResponse{OK: true, Path: root, Files: files, Warnings: warns})
}

func (h *Handlers) TopologyRenderConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RenderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, RenderConfigResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	req.NodeName = strings.TrimSpace(req.NodeName)
	if req.NodeName == "" {
		writeJSON(w, http.StatusBadRequest, RenderConfigResponse{OK: false, Error: "node name is required"})
		return
	}

	model, errs, warns := BuildTopologyModel(req.TopologyRequest)
	_, _, addrErrs, addrWarns := validateAddressing(req.TopologyRequest, model)
	errs = append(errs, addrErrs...)
	warns = append(warns, addrWarns...)
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, RenderConfigResponse{OK: false, Error: strings.Join(errs, "; "), Warnings: warns})
		return
	}

	plan, err := labplanner.BuildLabPlan(req.InfraCIDR, req.EdgeCIDR, model, toEdgeAttachments(req.EdgeLinks))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, RenderConfigResponse{OK: false, Error: err.Error(), Warnings: warns})
		return
	}

	nodeMap := map[string]labplanner.NodePlan{}
	for _, node := range plan.Nodes {
		nodeMap[node.Name] = node
	}
	node, ok := nodeMap[req.NodeName]
	if !ok {
		writeJSON(w, http.StatusBadRequest, RenderConfigResponse{OK: false, Error: "node not found in current topology: " + req.NodeName, Warnings: warns})
		return
	}
	if node.Role == "edge" {
		writeJSON(w, http.StatusBadRequest, RenderConfigResponse{OK: false, Error: "config preview is only available for infrastructure nodes", Warnings: warns})
		return
	}

	nodeLinks := map[string][]labplanner.LinkAssigned{}
	for _, link := range plan.Links {
		nodeLinks[link.A] = append(nodeLinks[link.A], link)
		nodeLinks[link.B] = append(nodeLinks[link.B], link)
	}

	tplPath := configTemplatePath(node.NodeType)
	body, err := configgenerator.RenderNodeConfig(tplPath, node, nodeLinks[node.Name], plan.Links, nodeMap, req.Monitoring.SNMP, req.Monitoring.GNMI)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, RenderConfigResponse{OK: false, Error: err.Error(), Warnings: warns})
		return
	}

	resp := RenderConfigResponse{
		OK:       true,
		Warnings: warns,
		NodeName: node.Name,
		NodeType: node.NodeType,
		Config:   body,
	}
	if node.NodeType == "frr" {
		resp.Daemons = configgenerator.RenderFRRDaemons(node.Protocols)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) TopologyDeploy(w http.ResponseWriter, r *http.Request) {
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

	labPath := filepath.Join(h.cfg.BaseDir, labName, "lab.clab.yml")
	if _, err := os.Stat(labPath); err != nil {
		writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: "lab file not found at " + labPath})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	output, err := runContainerlabLifecycle(ctx, "deploy", labPath, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: err.Error(), Output: string(output), Path: labPath})
		return
	}

	writeJSON(w, http.StatusOK, DeployResponse{OK: true, Output: string(output), Path: labPath})
}

func (h *Handlers) TopologyDestroy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DestroyRequest
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

	labPath := filepath.Join(h.cfg.BaseDir, labName, "lab.clab.yml")
	if _, err := os.Stat(labPath); err != nil {
		writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: "lab file not found at " + labPath})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	output, err := runContainerlabLifecycle(ctx, "destroy", labPath, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: err.Error(), Output: string(output), Path: labPath})
		return
	}

	writeJSON(w, http.StatusOK, DeployResponse{OK: true, Output: string(output), Path: labPath})
}

func (h *Handlers) TopologyDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DestroyRequest
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

	root := filepath.Join(h.cfg.BaseDir, labName)
	labPath := filepath.Join(root, "lab.clab.yml")
	var out []byte
	if _, err := os.Stat(labPath); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		output, err := runContainerlabLifecycle(ctx, "destroy", labPath, req.UseSudo)
		out = append(out, output...)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, DeployResponse{OK: false, Error: err.Error(), Output: string(output), Path: labPath})
			return
		}
	}

	if err := os.RemoveAll(root); err != nil {
		writeJSON(w, http.StatusInternalServerError, DeployResponse{OK: false, Error: "delete lab directory failed: " + err.Error(), Output: string(out), Path: root})
		return
	}
	if db, err := labstore.OpenLabDB(h.cfg.BaseDir); err == nil {
		_ = labstore.DeleteLab(db, labName)
		_ = db.Close()
	}

	msg := strings.TrimSpace(string(out))
	if msg != "" {
		msg += "\n"
	}
	msg += "deleted lab files and index entries"
	writeJSON(w, http.StatusOK, DeployResponse{OK: true, Output: msg, Path: root})
}

func runContainerlabLifecycle(ctx context.Context, action, labPath string, useSudo bool) ([]byte, error) {
	args := []string{"containerlab", action, "-t", labPath}
	if action == "deploy" {
		args = append(args, "--reconfigure")
	}
	if useSudo {
		args = append([]string{"sudo", "-E", "-n"}, args...)
	}

	cmd := execCommandContext(ctx, args[0], args[1:]...)
	clabBase := "/home/ubuntu/.clab-runs"
	_ = os.MkdirAll(clabBase, 0o755)
	cmd.Env = append(os.Environ(), "CLAB_LABDIR_BASE="+clabBase)
	return cmd.CombinedOutput()
}

func writeLabFiles(root, labName string, model labplanner.TopologyModel, plan labplanner.LabPlan, monitoring MonitoringConfig) ([]string, error) {
	var files []string
	yamlBody := configgenerator.RenderContainerlabYAML(labName, model, plan.Nodes, plan.Links, plan.EdgeHosts, monitoring.SNMP || monitoring.GNMI, monitoring.SNMP)
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
		tplPath := configTemplatePath(node.NodeType)
		body, err := configgenerator.RenderNodeConfig(tplPath, node, nodeLinks[node.Name], plan.Links, nodeMap, monitoring.SNMP, monitoring.GNMI)
		if err != nil {
			return files, err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return files, err
		}
		files = append(files, path)

		if node.NodeType == "frr" {
			daemonsPath := filepath.Join(cfgDir, node.Name+".daemons")
			daemonsBody := configgenerator.RenderFRRDaemons(node.Protocols)
			if err := os.WriteFile(daemonsPath, []byte(daemonsBody), 0o644); err != nil {
				return files, err
			}
			files = append(files, daemonsPath)
			if monitoring.SNMP {
				snmpdPath := filepath.Join(cfgDir, node.Name+".snmpd.conf")
				snmpdBody := configgenerator.FRRSNMPDConfig()
				if err := os.WriteFile(snmpdPath, []byte(snmpdBody), 0o644); err != nil {
					return files, err
				}
				files = append(files, snmpdPath)
			}
		}
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

	if monitoring.SNMP || monitoring.GNMI {
		monDir := filepath.Join(root, "monitoring")
		if err := os.MkdirAll(monDir, 0o755); err != nil {
			return files, err
		}
		promCfg := configgenerator.PrometheusConfig(labName, plan.Nodes, monitoring.SNMP, monitoring.GNMI)
		promPath := filepath.Join(monDir, "prometheus.yml")
		if err := os.WriteFile(promPath, []byte(promCfg), 0o644); err != nil {
			return files, err
		}
		files = append(files, promPath)

		grafanaCfg := configgenerator.GrafanaDatasource()
		grafanaPath := filepath.Join(monDir, "grafana-datasources.yml")
		if err := os.WriteFile(grafanaPath, []byte(grafanaCfg), 0o644); err != nil {
			return files, err
		}
		files = append(files, grafanaPath)

		if monitoring.GNMI {
			gnmiCfg := configgenerator.GNMIConfig(labName)
			gnmiPath := filepath.Join(monDir, "gnmic.yml")
			if err := os.WriteFile(gnmiPath, []byte(gnmiCfg), 0o644); err != nil {
				return files, err
			}
			files = append(files, gnmiPath)
		}
	}

	return files, nil
}

func configTemplatePath(nodeType string) string {
	name := "node.tmpl"
	if nodeType == "frr" {
		name = "node_frr.tmpl"
	}
	return resolveTemplatePath(filepath.Join("templates", "config", name))
}

func resolveTemplatePath(rel string) string {
	candidates := []string{
		rel,
		filepath.Join("..", rel),
		filepath.Join("..", "..", rel),
		filepath.Join("..", "..", "..", rel),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return rel
}

// BuildTopologyModel validates and normalizes UI topology input into planner-ready graph intent.
func BuildTopologyModel(req TopologyRequest) (labplanner.TopologyModel, []string, []string) {
	var errs []string
	var warns []string
	topology := strings.ToLower(strings.TrimSpace(req.Topology))
	if topology == "" {
		topology = "leaf-spine"
		warns = append(warns, "topology not set; defaulting to leaf-spine")
	}
	model := labplanner.TopologyModel{
		Topology:   topology,
		EdgeNodes:  maxInt(req.EdgeNodes, 0),
		EdgeFanout: maxInt(req.EdgeFanout, 1),
		Protocols:  labplanner.NormalizeProtocols(req.Protocols),
	}
	if req.MultiHoming {
		model.EdgeFanout = 2
	}
	nodeType := normalizeNodeType(req.NodeType)
	if nodeType == "invalid" {
		errs = append(errs, "node type must be arista or frr")
		nodeType = "arista"
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
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "spine" + itoa(i), Role: "spine", NodeType: nodeType})
		}
		for i := 1; i <= leaves; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "leaf" + itoa(i), Role: "leaf", NodeType: nodeType})
		}
		for i := 1; i <= spines; i++ {
			for j := 1; j <= leaves; j++ {
				model.Links = append(model.Links, labplanner.TopologyLink{A: "spine" + itoa(i), B: "leaf" + itoa(j)})
			}
		}
	case "full-mesh":
		n := clamp(req.NodeCount, 2, 256)
		if req.NodeCount < 2 {
			errs = append(errs, "node count must be >= 2")
		}
		for i := 1; i <= n; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "node" + itoa(i), Role: "mesh", NodeType: nodeType})
		}
		for i := 1; i <= n; i++ {
			for j := i + 1; j <= n; j++ {
				model.Links = append(model.Links, labplanner.TopologyLink{A: "node" + itoa(i), B: "node" + itoa(j)})
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
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "hub" + itoa(i), Role: "hub", NodeType: nodeType})
		}
		for i := 1; i <= spokes; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "spoke" + itoa(i), Role: "spoke", NodeType: nodeType})
		}
		for i := 1; i <= hubs; i++ {
			for j := 1; j <= spokes; j++ {
				model.Links = append(model.Links, labplanner.TopologyLink{A: "hub" + itoa(i), B: "spoke" + itoa(j)})
			}
		}
	case "custom":
		n := clamp(req.NodeCount, 1, 256)
		if req.NodeCount < 1 {
			errs = append(errs, "node count must be >= 1")
		}
		for i := 1; i <= n; i++ {
			model.Nodes = append(model.Nodes, labplanner.TopologyNode{Name: "node" + itoa(i), Role: "custom", NodeType: nodeType})
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
	if req.EdgeFanout < 0 {
		errs = append(errs, "edge fanout must be >= 0")
	}
	if req.MultiHoming {
		if topology != "leaf-spine" {
			errs = append(errs, "multi-homing is only supported for leaf-spine topologies")
		}
		if req.LeafCount < 3 {
			errs = append(errs, "multi-homing requires at least 3 leaf nodes")
		}
		if req.EdgeNodes < 1 {
			errs = append(errs, "multi-homing requires at least 1 edge node")
		}
	}

	return model, errs, warns
}

func normalizeNodeType(nodeType string) string {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "", "frr":
		return "frr"
	case "arista":
		return "arista"
	default:
		return "invalid"
	}
}

func nodeTypeIsArista(nodeType string) bool {
	return normalizeNodeType(nodeType) == "arista"
}

func aristaImagePresent(ctx context.Context) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := execCommandContext(checkCtx, "docker", "image", "inspect", requiredAristaImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "executable file not found") {
			return false, fmt.Errorf("docker command not found")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_ = out
			return false, nil
		}
		return false, fmt.Errorf("docker image inspect failed: %w", err)
	}
	return true, nil
}

func dockerImageLoad(ctx context.Context, archivePath string) ([]byte, error) {
	loadCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := execCommandContext(loadCtx, "docker", "image", "load", "-i", archivePath)
	return cmd.CombinedOutput()
}

func (h *Handlers) aristaImageArchivePath(origName string) string {
	base := "ceosimage.tar"
	trimmed := strings.TrimSpace(origName)
	if trimmed != "" {
		base = filepath.Base(trimmed)
	}
	return filepath.Join(h.cfg.BaseDir, ".images", base)
}

func (h *Handlers) saveAristaArchive(filename string, body io.Reader) (string, error) {
	dir := filepath.Join(h.cfg.BaseDir, ".images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create image archive dir failed: %w", err)
	}
	path := h.aristaImageArchivePath(filename)
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create image archive failed: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return "", fmt.Errorf("write image archive failed: %w", err)
	}
	return path, nil
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
	return []Check{{Name: "Topology inputs", Result: result, Detail: detail}}
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
