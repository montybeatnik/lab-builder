package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
	"github.com/montybeatnik/arista-lab/laber/labstore"
)

type walkthroughProfile struct {
	Item    WalkthroughCatalogItem
	Request TopologyRequest
}

var walkthroughProfiles = []walkthroughProfile{
	{
		Item: WalkthroughCatalogItem{
			ID:          "evpn-vxlan-stretched-l2-foundation",
			Name:        "EVPN/VXLAN Stretched L2 (Foundation)",
			Description: "Build a small 1-spine/2-leaf/2-edge fabric with IPv4 underlay BGP, then walk through EVPN/VXLAN overlay configuration and validation.",
			DurationMin: 30,
			Status:      "ready",
		},
		Request: TopologyRequest{
			Topology:   "leaf-spine",
			NodeType:   "frr",
			SpineCount: 1,
			LeafCount:  2,
			EdgeNodes:  2,
			InfraCIDR:  "10.0.0.0/24",
			EdgeCIDR:   "172.16.0.0/24",
			Protocols: labplanner.ProtocolSet{
				Global: []string{"bgp"},
				Roles: map[string][]string{
					"spine": {"bgp"},
					"leaf":  {"bgp"},
				},
			},
			LabName:    "walkthrough-evpn-vxlan-l2",
			Force:      true,
			Monitoring: MonitoringConfig{SNMP: false, GNMI: false},
		},
	},
	{
		Item: WalkthroughCatalogItem{
			ID:          "evpn-vxlan-multihoming",
			Name:        "EVPN Multihoming",
			Description: "Hands-on with edge multi-homing and validation for resilient L2 service delivery.",
			DurationMin: 35,
			Status:      "planned",
		},
	},
	{
		Item: WalkthroughCatalogItem{
			ID:          "evpn-vxlan-routing",
			Name:        "EVPN/VXLAN L3 Routing",
			Description: "Progress to distributed anycast gateway and inter-VNI routing with verification steps.",
			DurationMin: 40,
			Status:      "planned",
		},
	},
}

var runContainerlabLifecycleFn = runContainerlabLifecycle

func (h *Handlers) WalkthroughCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items := make([]WalkthroughCatalogItem, 0, len(walkthroughProfiles))
	for _, p := range walkthroughProfiles {
		items = append(items, p.Item)
	}
	writeJSON(w, http.StatusOK, WalkthroughCatalogResponse{OK: true, Items: items})
}

func (h *Handlers) WalkthroughPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughPreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughPreflightResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	profile, ok := walkthroughProfileByID(req.WalkthroughID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, WalkthroughPreflightResponse{OK: false, Error: "unknown walkthrough id"})
		return
	}
	if profile.Item.Status != "ready" {
		writeJSON(w, http.StatusBadRequest, WalkthroughPreflightResponse{OK: false, Error: "walkthrough is not ready yet"})
		return
	}
	deployed := h.deployedLabs(req.UseSudo, profile.Request.LabName)
	writeJSON(w, http.StatusOK, WalkthroughPreflightResponse{
		OK:            true,
		WalkthroughID: profile.Item.ID,
		LabName:       profile.Request.LabName,
		DeployedLabs:  deployed,
	})
}

func (h *Handlers) WalkthroughLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughLaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}

	profile, ok := walkthroughProfileByID(req.WalkthroughID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{OK: false, Error: "unknown walkthrough id"})
		return
	}
	if profile.Item.Status != "ready" {
		writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{OK: false, Error: "walkthrough is not ready yet"})
		return
	}

	deployed := h.deployedLabs(req.UseSudo, profile.Request.LabName)
	if len(deployed) > 0 && !req.ForceReplace {
		writeJSON(w, http.StatusOK, WalkthroughLaunchResponse{
			OK:              false,
			RequiresConfirm: true,
			WalkthroughID:   profile.Item.ID,
			LabName:         profile.Request.LabName,
			DestroyedLabs:   deployed,
			Error:           "existing deployed lab detected",
		})
		return
	}

	destroyed := make([]string, 0, len(deployed))
	for _, name := range deployed {
		labPath := filepath.Join(h.cfg.BaseDir, name, "lab.clab.yml")
		if _, err := os.Stat(labPath); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		output, err := runContainerlabLifecycleFn(ctx, "destroy", labPath, req.UseSudo)
		cancel()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{
				OK:            false,
				WalkthroughID: profile.Item.ID,
				LabName:       profile.Request.LabName,
				DestroyedLabs: destroyed,
				Error:         "failed to destroy existing lab " + name + ": " + err.Error(),
				Output:        string(output),
			})
			return
		}
		destroyed = append(destroyed, name)
	}

	planReq := profile.Request
	model, errs, warns := BuildTopologyModel(planReq)
	_, _, addrErrs, addrWarns := validateAddressing(planReq, model)
	errs = append(errs, addrErrs...)
	warns = append(warns, addrWarns...)
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{
			OK:            false,
			WalkthroughID: profile.Item.ID,
			LabName:       planReq.LabName,
			DestroyedLabs: destroyed,
			Error:         strings.Join(errs, "; "),
		})
		return
	}

	root := filepath.Join(h.cfg.BaseDir, planReq.LabName)
	_ = os.RemoveAll(root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, WalkthroughLaunchResponse{
			OK:            false,
			WalkthroughID: profile.Item.ID,
			LabName:       planReq.LabName,
			DestroyedLabs: destroyed,
			Error:         "mkdir failed: " + err.Error(),
		})
		return
	}

	plan, err := labplanner.BuildLabPlan(planReq.InfraCIDR, planReq.EdgeCIDR, model, toEdgeAttachments(planReq.EdgeLinks))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{
			OK:            false,
			WalkthroughID: profile.Item.ID,
			LabName:       planReq.LabName,
			DestroyedLabs: destroyed,
			Error:         err.Error(),
		})
		return
	}
	if _, err := writeLabFiles(root, planReq.LabName, model, plan, planReq.Monitoring); err != nil {
		writeJSON(w, http.StatusInternalServerError, WalkthroughLaunchResponse{
			OK:            false,
			WalkthroughID: profile.Item.ID,
			LabName:       planReq.LabName,
			DestroyedLabs: destroyed,
			Error:         err.Error(),
		})
		return
	}

	if db, err := labstore.OpenLabDB(h.cfg.BaseDir); err == nil {
		_ = labstore.UpsertLab(db, planReq.LabName, root)
		_ = labstore.SaveLabPlan(db, planReq.LabName, plan, model.Protocols)
		_ = db.Close()
	}

	labPath := filepath.Join(root, "lab.clab.yml")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	output, err := runContainerlabLifecycleFn(ctx, "deploy", labPath, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughLaunchResponse{
			OK:            false,
			WalkthroughID: profile.Item.ID,
			LabName:       planReq.LabName,
			DestroyedLabs: destroyed,
			Path:          labPath,
			Output:        string(output),
			Error:         err.Error(),
		})
		return
	}

	_ = warns
	writeJSON(w, http.StatusOK, WalkthroughLaunchResponse{
		OK:            true,
		WalkthroughID: profile.Item.ID,
		LabName:       planReq.LabName,
		DestroyedLabs: destroyed,
		Path:          labPath,
		Output:        string(output),
	})
}

func (h *Handlers) WalkthroughTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughTerminalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	req.LabName = strings.TrimSpace(req.LabName)
	req.NodeName = strings.TrimSpace(req.NodeName)
	req.Command = strings.TrimSpace(req.Command)
	if req.LabName == "" || req.NodeName == "" || req.Command == "" {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "labName, nodeName, and command are required"})
		return
	}
	if !isSafeName(req.LabName) || !isSafeName(req.NodeName) {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "labName and nodeName must be alphanumeric, dash, or underscore"})
		return
	}
	if len(req.Command) > 1000 {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "command too long"})
		return
	}

	labPath := filepath.Join(h.cfg.BaseDir, req.LabName, "lab.clab.yml")
	if _, err := os.Stat(labPath); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "lab file not found at " + labPath})
		return
	}

	tout := time.Duration(req.TimeoutSec) * time.Second
	if tout <= 0 || tout > 120*time.Second {
		tout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), tout)
	defer cancel()

	raw, err := runInspectFn(ctx, labPath, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "inspect failed: " + err.Error()})
		return
	}
	nodes, err := inspectNodesFromOutput(raw)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WalkthroughTerminalResponse{OK: false, Error: "parse inspect: " + err.Error()})
		return
	}
	var target string
	for _, n := range nodes {
		if shortNodeName(n.Name, req.LabName) != req.NodeName {
			continue
		}
		if strings.TrimSpace(n.ContainerID) != "" {
			target = strings.TrimSpace(n.ContainerID)
		} else {
			target = strings.TrimSpace(n.Name)
		}
		break
	}
	if target == "" {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: "node container not found for " + req.NodeName})
		return
	}

	out, err := runContainerCommandFn(ctx, target, req.UseSudo, "sh", "-lc", req.Command)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{
			OK:       false,
			Error:    "command failed: " + err.Error(),
			LabName:  req.LabName,
			NodeName: req.NodeName,
			Command:  req.Command,
			Output:   string(out),
		})
		return
	}
	writeJSON(w, http.StatusOK, WalkthroughTerminalResponse{
		OK:       true,
		LabName:  req.LabName,
		NodeName: req.NodeName,
		Command:  req.Command,
		Output:   string(out),
	})
}

func walkthroughProfileByID(id string) (walkthroughProfile, bool) {
	id = strings.TrimSpace(id)
	for _, p := range walkthroughProfiles {
		if p.Item.ID == id {
			return p, true
		}
	}
	return walkthroughProfile{}, false
}

func (h *Handlers) deployedLabs(useSudo bool, excludeLab string) []string {
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
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, lab := range labs {
		if lab.Name == "" || lab.Name == excludeLab {
			continue
		}
		labPath := filepath.Join(h.cfg.BaseDir, lab.Name, "lab.clab.yml")
		if _, err := os.Stat(labPath); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		raw, err := runInspectFn(ctx, labPath, useSudo)
		cancel()
		if err != nil {
			continue
		}
		nodes, err := inspectNodesFromOutput(raw)
		if err != nil || len(nodes) == 0 {
			continue
		}
		if !hasRunningNode(nodes) {
			continue
		}
		if !seen[lab.Name] {
			seen[lab.Name] = true
			out = append(out, lab.Name)
		}
	}
	sort.Strings(out)
	return out
}

func hasRunningNode(nodes []ContainerInfo) bool {
	for _, n := range nodes {
		state := strings.ToLower(strings.TrimSpace(n.State))
		status := strings.ToLower(strings.TrimSpace(n.Status))
		if state == "running" {
			return true
		}
		if strings.Contains(status, "up") {
			return true
		}
	}
	return false
}
