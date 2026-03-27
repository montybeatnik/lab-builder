package app

import (
	"context"
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

	"github.com/creack/pty"
	"github.com/montybeatnik/arista-lab/laber/labplanner"
	"github.com/montybeatnik/arista-lab/laber/labstore"
	"golang.org/x/net/websocket"
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

type walkthroughTerminalSession struct {
	ID       string
	LabName  string
	NodeName string
	Cmd      *exec.Cmd
	Stdin    io.WriteCloser

	mu      sync.Mutex
	output  []byte
	closed  bool
	lastUse time.Time
}

var walkthroughTerminalSessions = struct {
	mu sync.Mutex
	m  map[string]*walkthroughTerminalSession
}{
	m: map[string]*walkthroughTerminalSession{},
}

type wsTerminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

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

	target, err := h.resolveWalkthroughNodeTarget(ctx, req.LabName, req.NodeName, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalResponse{OK: false, Error: err.Error()})
		return
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

func (h *Handlers) WalkthroughTerminalStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughTerminalStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalStartResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	req.LabName = strings.TrimSpace(req.LabName)
	req.NodeName = strings.TrimSpace(req.NodeName)
	if req.LabName == "" || req.NodeName == "" {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalStartResponse{OK: false, Error: "labName and nodeName are required"})
		return
	}
	if !isSafeName(req.LabName) || !isSafeName(req.NodeName) {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalStartResponse{OK: false, Error: "labName and nodeName must be alphanumeric, dash, or underscore"})
		return
	}

	labPath := filepath.Join(h.cfg.BaseDir, req.LabName, "lab.clab.yml")
	if _, err := os.Stat(labPath); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalStartResponse{OK: false, Error: "lab file not found at " + labPath})
		return
	}

	tout := time.Duration(req.TimeoutSec) * time.Second
	if tout <= 0 || tout > 120*time.Second {
		tout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), tout)
	defer cancel()
	target, err := h.resolveWalkthroughNodeTarget(ctx, req.LabName, req.NodeName, req.UseSudo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalStartResponse{OK: false, Error: err.Error()})
		return
	}

	args := []string{"exec", "-i", target, "sh"}
	var cmd *exec.Cmd
	if req.UseSudo {
		cmd = exec.CommandContext(context.Background(), "sudo", append([]string{"-n", "docker"}, args...)...)
	} else {
		cmd = exec.CommandContext(context.Background(), "docker", args...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WalkthroughTerminalStartResponse{OK: false, Error: "stdin pipe: " + err.Error()})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WalkthroughTerminalStartResponse{OK: false, Error: "stdout pipe: " + err.Error()})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, WalkthroughTerminalStartResponse{OK: false, Error: "stderr pipe: " + err.Error()})
		return
	}
	if err := cmd.Start(); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalStartResponse{OK: false, Error: "start shell failed: " + err.Error()})
		return
	}

	sessionID := strconv.FormatInt(time.Now().UnixNano(), 36)
	sess := &walkthroughTerminalSession{
		ID:       sessionID,
		LabName:  req.LabName,
		NodeName: req.NodeName,
		Cmd:      cmd,
		Stdin:    stdin,
		output:   []byte(fmt.Sprintf("Connected to %s/%s\n", req.LabName, req.NodeName)),
		lastUse:  time.Now(),
	}
	walkthroughTerminalSessions.mu.Lock()
	walkthroughTerminalSessions.m[sessionID] = sess
	walkthroughTerminalSessions.mu.Unlock()

	go streamSessionOutput(sess, stdout)
	go streamSessionOutput(sess, stderr)
	go waitSessionExit(sess)

	writeJSON(w, http.StatusOK, WalkthroughTerminalStartResponse{
		OK:        true,
		SessionID: sessionID,
		LabName:   req.LabName,
		NodeName:  req.NodeName,
		Output:    "Shell ready\n",
	})
}

func (h *Handlers) WalkthroughTerminalWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughTerminalWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalPollResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	sess := getWalkthroughSession(strings.TrimSpace(req.SessionID))
	if sess == nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalPollResponse{OK: false, Error: "terminal session not found"})
		return
	}
	input := req.Input
	if input == "" {
		writeJSON(w, http.StatusOK, WalkthroughTerminalPollResponse{OK: true, SessionID: sess.ID})
		return
	}
	if !strings.HasSuffix(input, "\n") {
		input += "\n"
	}
	if _, err := io.WriteString(sess.Stdin, input); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalPollResponse{OK: false, Error: "write failed: " + err.Error()})
		return
	}
	markSessionUse(sess)
	writeJSON(w, http.StatusOK, WalkthroughTerminalPollResponse{OK: true, SessionID: sess.ID})
}

func (h *Handlers) WalkthroughTerminalPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughTerminalPollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalPollResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	sess := getWalkthroughSession(strings.TrimSpace(req.SessionID))
	if sess == nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalPollResponse{OK: false, Error: "terminal session not found"})
		return
	}
	markSessionUse(sess)
	output, next, closed := readSessionFromCursor(sess, req.Cursor)
	writeJSON(w, http.StatusOK, WalkthroughTerminalPollResponse{
		OK:         true,
		SessionID:  sess.ID,
		Output:     output,
		NextCursor: next,
		Closed:     closed,
	})
}

func (h *Handlers) WalkthroughTerminalClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WalkthroughTerminalCloseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, WalkthroughTerminalPollResponse{OK: false, Error: "bad JSON: " + err.Error()})
		return
	}
	sess := getWalkthroughSession(strings.TrimSpace(req.SessionID))
	if sess == nil {
		writeJSON(w, http.StatusOK, WalkthroughTerminalPollResponse{OK: true})
		return
	}
	closeSession(sess)
	writeJSON(w, http.StatusOK, WalkthroughTerminalPollResponse{OK: true, SessionID: sess.ID, Closed: true})
}

func (h *Handlers) WalkthroughTerminalWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Keep same auth/validation model as other walkthrough endpoints.
	labName := strings.TrimSpace(r.URL.Query().Get("labName"))
	nodeName := strings.TrimSpace(r.URL.Query().Get("nodeName"))
	sudoRaw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sudo")))
	useSudo := sudoRaw == "true" || sudoRaw == "1" || sudoRaw == "yes"
	if labName == "" || nodeName == "" {
		http.Error(w, "labName and nodeName are required", http.StatusBadRequest)
		return
	}
	if !isSafeName(labName) || !isSafeName(nodeName) {
		http.Error(w, "labName and nodeName must be alphanumeric, dash, or underscore", http.StatusBadRequest)
		return
	}
	labPath := filepath.Join(h.cfg.BaseDir, labName, "lab.clab.yml")
	if _, err := os.Stat(labPath); err != nil {
		http.Error(w, "lab file not found", http.StatusBadRequest)
		return
	}

	websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()
		// Resolve node target with a short timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		target, err := h.resolveWalkthroughNodeTarget(ctx, labName, nodeName, useSudo)
		cancel()
		if err != nil || target == "" {
			_ = websocket.JSON.Send(ws, wsTerminalMessage{Type: "error", Data: "node resolution failed"})
			return
		}

		args := []string{"exec", "-it", target, "sh"}
		var cmd *exec.Cmd
		if useSudo {
			cmd = exec.Command("sudo", append([]string{"-n", "docker"}, args...)...)
		} else {
			cmd = exec.Command("docker", args...)
		}
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		ptmx, err := pty.Start(cmd)
		if err != nil {
			_ = websocket.JSON.Send(ws, wsTerminalMessage{Type: "error", Data: "pty start failed: " + err.Error()})
			return
		}
		_ = pty.Setsize(ptmx, &pty.Winsize{Cols: 120, Rows: 36})
		defer func() {
			_ = ptmx.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}()

		_ = websocket.JSON.Send(ws, wsTerminalMessage{Type: "status", Data: "connected"})

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			for {
				n, err := ptmx.Read(buf)
				if n > 0 {
					_ = websocket.JSON.Send(ws, wsTerminalMessage{Type: "output", Data: string(buf[:n])})
				}
				if err != nil {
					return
				}
			}
		}()

		for {
			var msg wsTerminalMessage
			if err := websocket.JSON.Receive(ws, &msg); err != nil {
				return
			}
			switch msg.Type {
			case "input":
				if msg.Data == "" {
					continue
				}
				_, _ = io.WriteString(ptmx, msg.Data)
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(msg.Cols), Rows: uint16(msg.Rows)})
				}
			case "ping":
				_ = websocket.JSON.Send(ws, wsTerminalMessage{Type: "pong"})
			case "close":
				return
			}
			select {
			case <-done:
				return
			default:
			}
		}
	}).ServeHTTP(w, r)
}

func (h *Handlers) resolveWalkthroughNodeTarget(ctx context.Context, labName, nodeName string, useSudo bool) (string, error) {
	labPath := filepath.Join(h.cfg.BaseDir, labName, "lab.clab.yml")
	raw, err := runInspectFn(ctx, labPath, useSudo)
	if err != nil {
		return "", fmt.Errorf("inspect failed: %w", err)
	}
	nodes, err := inspectNodesFromOutput(raw)
	if err != nil {
		return "", fmt.Errorf("parse inspect: %w", err)
	}
	for _, n := range nodes {
		if shortNodeName(n.Name, labName) != nodeName {
			continue
		}
		if strings.TrimSpace(n.ContainerID) != "" {
			return strings.TrimSpace(n.ContainerID), nil
		}
		return strings.TrimSpace(n.Name), nil
	}
	return "", nil
}

func getWalkthroughSession(id string) *walkthroughTerminalSession {
	if id == "" {
		return nil
	}
	walkthroughTerminalSessions.mu.Lock()
	defer walkthroughTerminalSessions.mu.Unlock()
	return walkthroughTerminalSessions.m[id]
}

func markSessionUse(sess *walkthroughTerminalSession) {
	if sess == nil {
		return
	}
	sess.mu.Lock()
	sess.lastUse = time.Now()
	sess.mu.Unlock()
}

func readSessionFromCursor(sess *walkthroughTerminalSession, cursor int) (string, int, bool) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(sess.output) {
		cursor = len(sess.output)
	}
	chunk := string(sess.output[cursor:])
	return chunk, len(sess.output), sess.closed
}

func streamSessionOutput(sess *walkthroughTerminalSession, r io.Reader) {
	buf := make([]byte, 2048)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			appendSessionOutput(sess, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func appendSessionOutput(sess *walkthroughTerminalSession, data []byte) {
	if sess == nil || len(data) == 0 {
		return
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	sess.output = append(sess.output, data...)
	// Keep last ~256KB to avoid unbounded memory use.
	if len(sess.output) > 256*1024 {
		sess.output = sess.output[len(sess.output)-(256*1024):]
	}
}

func waitSessionExit(sess *walkthroughTerminalSession) {
	err := sess.Cmd.Wait()
	if err != nil {
		appendSessionOutput(sess, []byte("\n[terminal closed: "+err.Error()+"]\n"))
	} else {
		appendSessionOutput(sess, []byte("\n[terminal closed]\n"))
	}
	sess.mu.Lock()
	sess.closed = true
	sess.mu.Unlock()
}

func closeSession(sess *walkthroughTerminalSession) {
	if sess == nil {
		return
	}
	_ = sess.Stdin.Close()
	if sess.Cmd.Process != nil {
		_ = sess.Cmd.Process.Kill()
	}
	walkthroughTerminalSessions.mu.Lock()
	delete(walkthroughTerminalSessions.m, sess.ID)
	walkthroughTerminalSessions.mu.Unlock()
	sess.mu.Lock()
	sess.closed = true
	sess.mu.Unlock()
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
