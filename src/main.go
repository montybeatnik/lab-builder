package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/montybeatnik/arista-lab/laber/pkgs/arista"
	"github.com/montybeatnik/arista-lab/laber/pkgs/renderer"
)

type InspectResult map[string][]ContainerInfo

type ContainerInfo struct {
	LabName     string `json:"lab_name"`
	LabPath     string `json:"labPath"`
	AbsLabPath  string `json:"absLabPath"`
	Name        string `json:"name"`
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Status      string `json:"status"`
	IPv4        string `json:"ipv4_address"`
	IPv6        string `json:"ipv6_address"`
	Owner       string `json:"owner"`
}

// ======= Config =======

type serverCfg struct {
	Listen  string
	BaseDir string // lab files must live under here
}

func (c serverCfg) sanitizeLabPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("lab file required")
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(c.BaseDir, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	baseAbs, _ := filepath.Abs(c.BaseDir)
	if abs != baseAbs && !strings.HasPrefix(abs, baseAbs+string(os.PathSeparator)) {
		return "", errors.New("lab file must be under basedir")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", errors.New("lab file not found")
	}
	if info.IsDir() {
		abs = filepath.Join(abs, "lab.clab.yml")
		info, err = os.Stat(abs)
		if err != nil || info.IsDir() {
			return "", errors.New("lab file not found")
		}
	}
	return abs, nil
}

// ======= Handlers =======

type pageData struct {
	BaseDir string
	Page    string
}

//go:embed web/templates/*.tmpl
var tplFS embed.FS

//go:embed web/static/*
var staticFS embed.FS

// parse templates (explicit order to be clear)
func makeTemplate() *template.Template {
	return template.Must(template.ParseFS(
		tplFS,
		"web/templates/layout.tmpl",
		"web/templates/index.tmpl",
		"web/templates/lab.tmpl",
		"web/templates/viewer.tmpl",
	))
}

func indexHandler(cfg serverCfg, t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		data := pageData{
			BaseDir: cfg.BaseDir,
			Page:    "index",
		}
		if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
			// nothing has been written to the client yet → safe to send an error
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = buf.WriteTo(w)
	}
}

func labHandler(cfg serverCfg, t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		data := pageData{
			BaseDir: cfg.BaseDir,
			Page:    "health",
		}
		if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = buf.WriteTo(w)
	}
}

func viewerHandler(cfg serverCfg, t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		data := pageData{
			BaseDir: cfg.BaseDir,
			Page:    "viewer",
		}
		if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = buf.WriteTo(w)
	}
}

type inspectReq struct {
	Lab        string `json:"lab"`
	UseSudo    bool   `json:"sudo"`
	TimeoutSec int    `json:"timeoutSec"`
}

type inspectResp struct {
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	LabKey  string          `json:"labKey,omitempty"`
	Nodes   []ContainerInfo `json:"nodes,omitempty"`
	RawJSON json.RawMessage `json:"rawJson,omitempty"`
}

func runInspect(ctx context.Context, labPath string, useSudo bool) ([]byte, error) {
	args := []string{"containerlab", "inspect", "-t", labPath, "--format", "json"}
	if useSudo {
		args = append([]string{"sudo", "-n"}, args...)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func inspectHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow POST; if you clicked and nothing happened before, it’s
		// often script not loaded or method mismatch. We enforce POST here.
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req inspectReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, inspectResp{OK: false, Error: "bad JSON: " + err.Error()})
			return
		}

		labAbs, err := cfg.sanitizeLabPath(req.Lab)
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
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func runCmdHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cmds := []string{"show bgp summary"}
		url := "https://172.20.20.9/command-api"
		client := arista.NewEosClient(url)
		tmplPath := "templates/eapi_payload.tmpl"
		fmt.Println("rendering template...")
		body, err := renderer.RenderTemplate(tmplPath, renderer.PayloadData{
			Method:  "runCmds",
			Version: 1,
			Format:  "json",
			Cmds:    cmds,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("running cmd...")
		var resp map[string]any
		_ = client.Run(body, &resp)
	}
}

// ----- Health API -----

type HealthReq struct {
	Lab        string `json:"lab"`
	UseSudo    bool   `json:"sudo"`
	TimeoutSec int    `json:"timeoutSec"`
	User       string `json:"user"`
	Pass       string `json:"pass"`
}

type HealthCheck struct {
	Name   string `json:"name"`
	Result string `json:"result"` // PASS|WARN|FAIL
	Detail string `json:"detail,omitempty"`
}

type NodeHealth struct {
	Name   string        `json:"name"`
	IP     string        `json:"ip"`
	Checks []HealthCheck `json:"checks"`
}

type HealthResp struct {
	OK    bool         `json:"ok"`
	Error string       `json:"error,omitempty"`
	Nodes []NodeHealth `json:"nodes,omitempty"`
}

func ceosNodesFromInspect(out []byte) (nodes []ContainerInfo, err error) {
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
		break // first (and only) lab key
	}
	return
}

// simple helpers

func cidrIP(s string) string {
	// "172.20.20.7/24" -> "172.20.20.7"
	if i := strings.IndexByte(s, '/'); i > 0 {
		return s[:i]
	}
	return s
}

func onlyNonEmpty(ss []string) (out []string) {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return
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
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // lab only
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

func healthHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req HealthReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, HealthResp{OK: false, Error: "bad JSON: " + err.Error()})
			return
		}
		labAbs, err := cfg.sanitizeLabPath(req.Lab)
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
		nodes, err := ceosNodesFromInspect(out)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, HealthResp{OK: false, Error: "parse inspect: " + err.Error()})
			return
		}
		if len(nodes) == 0 {
			writeJSON(w, http.StatusOK, HealthResp{OK: true, Nodes: nil})
			return
		}

		// The checks we’ll run per node
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

				ip := cidrIP(n.IPv4)
				cx, cancel := context.WithTimeout(r.Context(), tout)
				defer cancel()

				status, body, err := eapiRun(cx, ip, req.User, req.Pass, checkCmds, "json")
				h := NodeHealth{Name: n.Name, IP: ip}

				if err != nil || status < 200 || status >= 300 {
					h.Checks = append(h.Checks, HealthCheck{
						Name:   "eAPI reachability",
						Result: "FAIL",
						Detail: fmt.Sprintf("status=%d err=%v", status, err),
					})
					ch <- item{i: i, nh: h}
					return
				}

				// Parse minimal fields from body
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
					h.Checks = append(h.Checks, HealthCheck{Name: "parse", Result: "WARN", Detail: "bad JSON from eAPI"})
					ch <- item{i: i, nh: h}
					return
				}
				if rp.Error != nil {
					h.Checks = append(h.Checks, HealthCheck{Name: "eAPI error", Result: "FAIL", Detail: rp.Error.Message})
					ch <- item{i: i, nh: h}
					return
				}
				if len(rp.Result) == 0 {
					h.Checks = append(h.Checks, HealthCheck{Name: "parse", Result: "WARN", Detail: "empty eAPI result"})
					ch <- item{i: i, nh: h}
					return
				}

				// 1) EVPN neighbors established?
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
				h.Checks = append(h.Checks, HealthCheck{
					Name:   "EVPN neighbors",
					Result: map[bool]string{true: "PASS", false: "FAIL"}[pass1],
					Detail: map[bool]string{true: "at least one Established", false: "none Established"}[pass1],
				})

				// 2) VTEPs learnt?
				pass2 := false
				if len(rp.Result) >= 2 {
					if r1, ok := rp.Result[1].(map[string]any); ok {
						if m, ok := r1["vteps"].([]any); ok && len(m) > 0 {
							pass2 = true
						}
					}
				}
				h.Checks = append(h.Checks, HealthCheck{
					Name:   "VXLAN VTEPs",
					Result: map[bool]string{true: "PASS", false: "WARN"}[pass2],
					Detail: map[bool]string{true: "remote VTEPs present", false: "no remote VTEPs"}[pass1],
				})

				// 3) Any MAC/IP routes?
				pass3 := false
				if len(rp.Result) >= 3 {
					if r2, ok := rp.Result[2].(map[string]any); ok {
						if routes, ok := r2["routes"].([]any); ok && len(routes) > 0 {
							pass3 = true
						}
					}
				}
				h.Checks = append(h.Checks, HealthCheck{
					Name:   "EVPN MAC/IP",
					Result: map[bool]string{true: "PASS", false: "WARN"}[pass3],
					Detail: map[bool]string{true: "mac-ip entries found", false: "no mac-ip entries"}[pass1],
				})

				ch <- item{i: i, nh: h}
			}(i, n)
		}
		go func() { wg.Wait(); close(ch) }()

		results := make([]NodeHealth, len(nodes))
		for it := range ch {
			results[it.i] = it.nh
		}
		writeJSON(w, http.StatusOK, HealthResp{OK: true, Nodes: results})
	}
}

func main() {
	cfg := serverCfg{
		Listen:  ":8080",
		BaseDir: "/home/ubuntu/lab",
	}

	// Templates
	t := makeTemplate()

	// Static files (embed FS subdir)
	static, _ := fs.Sub(staticFS, "web/static")
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	// Pages & API
	mux.HandleFunc("/", indexHandler(cfg, t))
	mux.HandleFunc("/lab", labHandler(cfg, t))
	mux.HandleFunc("/viewer", viewerHandler(cfg, t))
	mux.HandleFunc("/labs", labsHandler(cfg))
	mux.HandleFunc("/labplan", labPlanHandler(cfg))
	mux.HandleFunc("/inspect", inspectHandler(cfg))
	mux.HandleFunc("/runcmd", runCmdHandler(cfg))
	mux.HandleFunc("/health", healthHandler(cfg))
	mux.HandleFunc("/topology/validate", topologyHandler)
	mux.HandleFunc("/topology/build", topologyBuildHandler(cfg))
	mux.HandleFunc("/topology/deploy", topologyDeployHandler(cfg))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	println("listening on", cfg.Listen, "basedir:", cfg.BaseDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
