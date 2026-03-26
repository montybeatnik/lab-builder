package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectHandler_MethodNotAllowed(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodGet, "/inspect", nil)
	rec := httptest.NewRecorder()
	h.Inspect(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestInspectHandler_BadJSON(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodPost, "/inspect", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	h.Inspect(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Health(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHealthHandler_BadJSON(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodPost, "/health", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	h.Health(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestTopologyRenderConfig_MethodNotAllowed(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodGet, "/topology/render-config", nil)
	rec := httptest.NewRecorder()
	h.TopologyRenderConfig(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestTopologyRenderConfig_BadJSON(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodPost, "/topology/render-config", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	h.TopologyRenderConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestTopologyRenderConfig_RendersSpineInterfaces(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	body := `{
		"topology":"leaf-spine",
		"nodeType":"frr",
		"spineCount":2,
		"leafCount":2,
		"edgeNodes":0,
		"infraCidr":"10.0.0.0/20",
		"edgeCidr":"172.16.0.0/24",
		"protocols":{"global":["bgp"],"roles":{"spine":["evpn"],"leaf":["evpn"]}},
		"monitoring":{"snmp":false,"gnmi":false},
		"nodeName":"spine2"
	}`
	req := httptest.NewRequest(http.MethodPost, "/topology/render-config", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.TopologyRenderConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp RenderConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response: %#v", resp)
	}
	if !strings.Contains(resp.Config, "interface eth1") || !strings.Contains(resp.Config, "interface eth2") {
		t.Fatalf("expected spine interfaces in config:\n%s", resp.Config)
	}
	if !strings.Contains(resp.Config, "router bgp 65000") {
		t.Fatalf("expected bgp stanza in config:\n%s", resp.Config)
	}
}

func TestLabNodes_ReturnsConfigNames(t *testing.T) {
	base := t.TempDir()
	labDir := filepath.Join(base, "demo")
	cfgDir := filepath.Join(labDir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "leaf1.cfg"), []byte("hostname leaf1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "spine2.cfg"), []byte("hostname spine2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodPost, "/lab/nodes", strings.NewReader(`{"lab":"demo/lab.clab.yml"}`))
	rec := httptest.NewRecorder()
	h.LabNodes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp LabNodesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Nodes) != 2 || resp.Nodes[0] != "leaf1" || resp.Nodes[1] != "spine2" {
		t.Fatalf("unexpected nodes: %#v", resp.Nodes)
	}
}

func TestLabNodeConfig_ReadsFiles(t *testing.T) {
	base := t.TempDir()
	labDir := filepath.Join(base, "demo")
	cfgDir := filepath.Join(labDir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "spine2.cfg"), []byte("interface eth1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "spine2.daemons"), []byte("bgpd=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodPost, "/lab/config", strings.NewReader(`{"lab":"demo/lab.clab.yml","nodeName":"spine2"}`))
	rec := httptest.NewRecorder()
	h.LabNodeConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp LabNodeConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp.Config, "interface eth1") {
		t.Fatalf("expected config body: %#v", resp)
	}
	if !strings.Contains(resp.Daemons, "bgpd=yes") {
		t.Fatalf("expected daemons body: %#v", resp)
	}
}

func TestLabNodeConfig_IncludesStartupExec(t *testing.T) {
	base := t.TempDir()
	labDir := filepath.Join(base, "demo")
	cfgDir := filepath.Join(labDir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	labYAML := `name: demo
topology:
  nodes:
    leaf1:
      kind: linux
      exec:
        - ip link add br0 type bridge
        - ip link set dev eth3 master br0
`
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte(labYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "leaf1.cfg"), []byte("hostname leaf1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodPost, "/lab/config", strings.NewReader(`{"lab":"demo/lab.clab.yml","nodeName":"leaf1"}`))
	rec := httptest.NewRecorder()
	h.LabNodeConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp LabNodeConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp.Startup, "ip link add br0 type bridge") {
		t.Fatalf("expected startup exec body: %#v", resp)
	}
}

func TestLabs_IncludesFilesystemLabsWithoutIndexEntry(t *testing.T) {
	base := t.TempDir()
	labDir := filepath.Join(base, "older-lab")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte("name: older-lab\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodGet, "/labs", nil)
	rec := httptest.NewRecorder()
	h.Labs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp LabsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Labs) != 1 || resp.Labs[0].Name != "older-lab" {
		t.Fatalf("expected filesystem lab in response: %#v", resp.Labs)
	}
}

func TestLabs_DetectsLabNodeType(t *testing.T) {
	base := t.TempDir()
	labDir := filepath.Join(base, "frr-lab")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatal(err)
	}
	labYAML := `name: frr-lab
topology:
  nodes:
    leaf1:
      kind: linux
      image: quay.io/frrouting/frr:9.1.3
      binds:
        - configs/leaf1.cfg:/etc/frr/frr.conf
`
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte(labYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodGet, "/labs", nil)
	rec := httptest.NewRecorder()
	h.Labs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp LabsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Labs) != 1 || resp.Labs[0].NodeType != "frr" {
		t.Fatalf("expected detected frr node type, got %#v", resp.Labs)
	}
}

func TestLabs_FallsBackWhenIndexIsNotUsable(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, ".lab-index.sqlite"), []byte("not-a-sqlite-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	labDir := filepath.Join(base, "frr-lab")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte("name: frr-lab\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodGet, "/labs", nil)
	rec := httptest.NewRecorder()
	h.Labs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp LabsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || len(resp.Labs) != 1 || resp.Labs[0].Name != "frr-lab" {
		t.Fatalf("expected filesystem fallback labs, got %#v", resp)
	}
}

func TestTopologyDestroy_MethodNotAllowed(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodGet, "/topology/destroy", nil)
	rec := httptest.NewRecorder()
	h.TopologyDestroy(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestTopologyDestroy_RunsContainerlabDestroy(t *testing.T) {
	base := t.TempDir()
	labDir := filepath.Join(base, "demo")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.clab.yml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origExec := execCommandContext
	var gotCommand []string
	execCommandContext = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		gotCommand = append([]string{command}, args...)
		return exec.CommandContext(ctx, "sh", "-c", "printf 'destroy ok'")
	}
	defer func() { execCommandContext = origExec }()

	h := NewHandlers(Config{BaseDir: base}, nil)
	req := httptest.NewRequest(http.MethodPost, "/topology/destroy", strings.NewReader(`{"labName":"demo","sudo":true}`))
	rec := httptest.NewRecorder()
	h.TopologyDestroy(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp DeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response: %#v", resp)
	}
	if !strings.Contains(resp.Output, "destroy ok") {
		t.Fatalf("expected destroy output, got %#v", resp)
	}
	if len(gotCommand) == 0 || gotCommand[0] != "sudo" {
		t.Fatalf("expected sudo-wrapped command, got %#v", gotCommand)
	}
	joined := strings.Join(gotCommand, " ")
	if !strings.Contains(joined, "containerlab destroy") {
		t.Fatalf("expected containerlab destroy invocation, got %q", joined)
	}
}
