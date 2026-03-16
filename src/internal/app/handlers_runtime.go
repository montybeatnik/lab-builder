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
	db, err := labstore.OpenLabDB(h.cfg.BaseDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LabsResponse{OK: false, Error: err.Error()})
		return
	}
	defer db.Close()

	labs, err := labstore.ListLabs(db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LabsResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, LabsResponse{OK: true, Labs: labs})
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
	nodes, err := CEOSNodesFromInspect(out)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, HealthResp{OK: false, Error: "parse inspect: " + err.Error()})
		return
	}
	if len(nodes) == 0 {
		writeJSON(w, http.StatusOK, HealthResp{OK: true, Nodes: nil})
		return
	}

	checkCmds := []string{
		"show bgp evpn summary",
		"show vxlan vtep",
		"show bgp evpn route-type mac-ip",
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

			ip := CIDRIP(n.IPv4)
			cx, cancel := context.WithTimeout(r.Context(), tout)
			defer cancel()

			status, body, err := eapiRun(cx, ip, req.User, req.Pass, checkCmds, "json")
			hh := NodeHealth{Name: n.Name, IP: ip}

			if err != nil || status < 200 || status >= 300 {
				hh.Checks = append(hh.Checks, HealthCheck{
					Name:   "eAPI reachability",
					Result: "FAIL",
					Detail: fmt.Sprintf("status=%d err=%v", status, err),
				})
				ch <- item{i: i, nh: hh}
				return
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
				ch <- item{i: i, nh: hh}
				return
			}
			if rp.Error != nil {
				hh.Checks = append(hh.Checks, HealthCheck{Name: "eAPI error", Result: "FAIL", Detail: rp.Error.Message})
				ch <- item{i: i, nh: hh}
				return
			}
			if len(rp.Result) == 0 {
				hh.Checks = append(hh.Checks, HealthCheck{Name: "parse", Result: "WARN", Detail: "empty eAPI result"})
				ch <- item{i: i, nh: hh}
				return
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
