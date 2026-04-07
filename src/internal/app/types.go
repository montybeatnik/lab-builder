package app

import (
	"encoding/json"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
	"github.com/montybeatnik/arista-lab/laber/labstore"
)

type InspectResult map[string][]ContainerInfo

// ContainerInfo mirrors `containerlab inspect --format json` node entries.
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

type pageData struct {
	BaseDir string
	Page    string
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

// HealthReq drives active control-plane/data-plane health checks for a selected lab.
type HealthReq struct {
	Lab        string `json:"lab"`
	UseSudo    bool   `json:"sudo"`
	TimeoutSec int    `json:"timeoutSec"`
	User       string `json:"user"`
	Pass       string `json:"pass"`
}

// HealthCheck is one named check result surfaced in Health UI and APIs.
type HealthCheck struct {
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

// NodeHealth groups check results by node for easier per-device troubleshooting.
type NodeHealth struct {
	Name   string        `json:"name"`
	IP     string        `json:"ip"`
	Checks []HealthCheck `json:"checks"`
}

// HealthResp is the API response shape for `/health`.
type HealthResp struct {
	OK    bool         `json:"ok"`
	Error string       `json:"error,omitempty"`
	Nodes []NodeHealth `json:"nodes,omitempty"`
}

// LabsResponse powers the Lab Manager listing with indexed + filesystem labs.
type LabsResponse struct {
	OK    bool                 `json:"ok"`
	Error string               `json:"error,omitempty"`
	Labs  []labstore.LabRecord `json:"labs,omitempty"`
}

// LabPlanResponse exposes persisted planner output for viewer and walkthrough pages.
type LabPlanResponse struct {
	OK        bool               `json:"ok"`
	Error     string             `json:"error,omitempty"`
	Nodes     []NodePlanJSON     `json:"nodes,omitempty"`
	Links     []LinkAssignedJSON `json:"links,omitempty"`
	Protocols ProtocolSetJSON    `json:"protocols,omitempty"`
}

type NodePlanJSON struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	ASN        int    `json:"asn"`
	Loopback   string `json:"loopback"`
	EdgeIP     string `json:"edgeIp"`
	EdgePrefix int    `json:"edgePrefix"`
}

type LinkAssignedJSON struct {
	A   string `json:"a"`
	B   string `json:"b"`
	AIf string `json:"aIf"`
	BIf string `json:"bIf"`
	AIP string `json:"aIp,omitempty"`
	BIP string `json:"bIp,omitempty"`
}

type ProtocolSetJSON struct {
	Global []string            `json:"global"`
	Roles  map[string][]string `json:"roles"`
}

// TopologyRequest carries user topology intent from Build/Walkthrough pages.
type TopologyRequest struct {
	Topology    string                 `json:"topology"`
	NodeType    string                 `json:"nodeType"`
	NodeCount   int                    `json:"nodeCount"`
	LeafCount   int                    `json:"leafCount"`
	SpineCount  int                    `json:"spineCount"`
	HubCount    int                    `json:"hubCount"`
	SpokeCount  int                    `json:"spokeCount"`
	EdgeNodes   int                    `json:"edgeNodes"`
	EdgeFanout  int                    `json:"edgeFanout"`
	MultiHoming bool                   `json:"multiHoming"`
	InfraCIDR   string                 `json:"infraCidr"`
	EdgeCIDR    string                 `json:"edgeCidr"`
	CustomLinks []LinkInput            `json:"customLinks"`
	EdgeLinks   []EdgeLinkInput        `json:"edgeLinks"`
	Traffic     []Traffic              `json:"traffic"`
	LabName     string                 `json:"labName"`
	Force       bool                   `json:"force"`
	UseSudo     bool                   `json:"sudo"`
	Protocols   labplanner.ProtocolSet `json:"protocols"`
	Monitoring  MonitoringConfig       `json:"monitoring"`
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

// MonitoringConfig toggles optional telemetry sidecars in generated labs.
type MonitoringConfig struct {
	SNMP bool `json:"snmp"`
	GNMI bool `json:"gnmi"`
}

type Check struct {
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

// AddressSummary captures capacity/consumption estimates for infra and edge CIDRs.
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

// TopologyResponse returns validation/build-readiness results for a topology request.
type TopologyResponse struct {
	OK        bool                     `json:"ok"`
	Errors    []string                 `json:"errors,omitempty"`
	Warnings  []string                 `json:"warnings,omitempty"`
	Checks    []Check                  `json:"checks,omitempty"`
	Model     labplanner.TopologyModel `json:"model,omitempty"`
	Address   AddressSummary           `json:"address,omitempty"`
	CanBuild  bool                     `json:"canBuild"`
	Notes     []string                 `json:"notes,omitempty"`
	RawTarget json.RawMessage          `json:"rawTarget,omitempty"`
}

type BuildResponse struct {
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Path     string   `json:"path,omitempty"`
	Files    []string `json:"files,omitempty"`
}

// AristaImageStatusResponse reports whether the required cEOS image is available locally.
type AristaImageStatusResponse struct {
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	Present       bool   `json:"present"`
	RequiredImage string `json:"requiredImage,omitempty"`
	ArchivePath   string `json:"archivePath,omitempty"`
}

// AristaImageUploadResponse reports upload + docker load results for a cEOS image archive.
type AristaImageUploadResponse struct {
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	RequiredImage string `json:"requiredImage,omitempty"`
	ArchivePath   string `json:"archivePath,omitempty"`
	Output        string `json:"output,omitempty"`
}

// DeployRequest asks the runtime to deploy a generated lab with optional sudo/force flags.
type DeployRequest struct {
	LabName string `json:"labName"`
	UseSudo bool   `json:"sudo"`
	Force   bool   `json:"force"`
}

// DeployResponse reports deploy/destroy command output back to the UI.
type DeployResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Output string `json:"output,omitempty"`
	Path   string `json:"path,omitempty"`
}

type DestroyRequest struct {
	LabName string `json:"labName"`
	UseSudo bool   `json:"sudo"`
}

// RenderConfigRequest asks the backend to render one node config without writing files.
type RenderConfigRequest struct {
	TopologyRequest
	NodeName string `json:"nodeName"`
}

// RenderConfigResponse returns generated node config snippets for preview/debug.
type RenderConfigResponse struct {
	OK       bool     `json:"ok"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	NodeName string   `json:"nodeName,omitempty"`
	NodeType string   `json:"nodeType,omitempty"`
	Config   string   `json:"config,omitempty"`
	Daemons  string   `json:"daemons,omitempty"`
}

type LabNodesRequest struct {
	Lab string `json:"lab"`
}

type LabNodesResponse struct {
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	Nodes []string `json:"nodes,omitempty"`
}

type LabNodeConfigRequest struct {
	Lab      string `json:"lab"`
	NodeName string `json:"nodeName"`
}

type LabNodeConfigResponse struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	Config   string `json:"config,omitempty"`
	Daemons  string `json:"daemons,omitempty"`
	Startup  string `json:"startup,omitempty"`
}

// LiveTopologyRequest asks the backend for current link and optional peering state.
type LiveTopologyRequest struct {
	LabName      string `json:"labName"`
	UseSudo      bool   `json:"sudo"`
	ShowPeerings bool   `json:"showPeerings"`
}

type LiveEndpointStatus struct {
	Node  string `json:"node"`
	Iface string `json:"iface"`
	State string `json:"state"`
}

// LiveLinkStatus represents one topology edge with endpoint-level operational status.
type LiveLinkStatus struct {
	A         string               `json:"a"`
	B         string               `json:"b"`
	AIf       string               `json:"aIf"`
	BIf       string               `json:"bIf"`
	State     string               `json:"state"`
	Endpoints []LiveEndpointStatus `json:"endpoints,omitempty"`
}

type LiveSummary struct {
	Up      int `json:"up"`
	Down    int `json:"down"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

type LivePeeringStatus struct {
	A       string `json:"a"`
	B       string `json:"b"`
	AfiSafi string `json:"afiSafi"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
}

// LiveTopologyResponse is the snapshot payload for the live topology page.
type LiveTopologyResponse struct {
	OK       bool                `json:"ok"`
	Error    string              `json:"error,omitempty"`
	LabName  string              `json:"labName,omitempty"`
	Nodes    []NodePlanJSON      `json:"nodes,omitempty"`
	Links    []LiveLinkStatus    `json:"links,omitempty"`
	Peerings []LivePeeringStatus `json:"peerings,omitempty"`
	Summary  LiveSummary         `json:"summary,omitempty"`
	PolledAt string              `json:"polledAt,omitempty"`
}

// TrafficRequest asks the backend to run a synthetic edge-to-edge ping test.
type TrafficRequest struct {
	LabName string `json:"labName"`
	UseSudo bool   `json:"sudo"`
	Source  string `json:"source"`
	Target  string `json:"target"`
	Count   int    `json:"count"`
}

type TrafficResponse struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	LabName  string `json:"labName,omitempty"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target,omitempty"`
	TargetIP string `json:"targetIp,omitempty"`
	Command  string `json:"command,omitempty"`
	Output   string `json:"output,omitempty"`
}

// WalkthroughCatalogItem describes one guided lab option shown in the catalog.
type WalkthroughCatalogItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DurationMin int    `json:"durationMin"`
	Status      string `json:"status"`
}

type WalkthroughCatalogResponse struct {
	OK    bool                     `json:"ok"`
	Error string                   `json:"error,omitempty"`
	Items []WalkthroughCatalogItem `json:"items,omitempty"`
}

// WalkthroughPreflightRequest validates whether a walkthrough can be launched safely.
type WalkthroughPreflightRequest struct {
	WalkthroughID string `json:"walkthroughId"`
	UseSudo       bool   `json:"sudo"`
}

type WalkthroughPreflightResponse struct {
	OK            bool     `json:"ok"`
	Error         string   `json:"error,omitempty"`
	WalkthroughID string   `json:"walkthroughId,omitempty"`
	LabName       string   `json:"labName,omitempty"`
	DeployedLabs  []string `json:"deployedLabs,omitempty"`
}

// WalkthroughLaunchRequest starts a walkthrough deployment, optionally replacing running labs.
type WalkthroughLaunchRequest struct {
	WalkthroughID string `json:"walkthroughId"`
	UseSudo       bool   `json:"sudo"`
	ForceReplace  bool   `json:"forceReplace"`
}

type WalkthroughLaunchResponse struct {
	OK              bool     `json:"ok"`
	Error           string   `json:"error,omitempty"`
	RequiresConfirm bool     `json:"requiresConfirm,omitempty"`
	WalkthroughID   string   `json:"walkthroughId,omitempty"`
	LabName         string   `json:"labName,omitempty"`
	DestroyedLabs   []string `json:"destroyedLabs,omitempty"`
	Path            string   `json:"path,omitempty"`
	Output          string   `json:"output,omitempty"`
}

// WalkthroughTerminalRequest executes one-shot commands on a walkthrough node container.
type WalkthroughTerminalRequest struct {
	LabName    string `json:"labName"`
	NodeName   string `json:"nodeName"`
	Command    string `json:"command"`
	UseSudo    bool   `json:"sudo"`
	TimeoutSec int    `json:"timeoutSec"`
}

type WalkthroughTerminalResponse struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	LabName  string `json:"labName,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	Command  string `json:"command,omitempty"`
	Output   string `json:"output,omitempty"`
}

// WalkthroughTerminalStartRequest opens an interactive shell session for walkthrough nodes.
type WalkthroughTerminalStartRequest struct {
	LabName    string `json:"labName"`
	NodeName   string `json:"nodeName"`
	UseSudo    bool   `json:"sudo"`
	TimeoutSec int    `json:"timeoutSec"`
}

type WalkthroughTerminalStartResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	LabName   string `json:"labName,omitempty"`
	NodeName  string `json:"nodeName,omitempty"`
	Output    string `json:"output,omitempty"`
}

type WalkthroughTerminalWriteRequest struct {
	SessionID string `json:"sessionId"`
	Input     string `json:"input"`
}

type WalkthroughTerminalPollRequest struct {
	SessionID string `json:"sessionId"`
	Cursor    int    `json:"cursor"`
}

// WalkthroughTerminalPollResponse streams incremental output from interactive walkthrough sessions.
type WalkthroughTerminalPollResponse struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Output     string `json:"output,omitempty"`
	NextCursor int    `json:"nextCursor,omitempty"`
	Closed     bool   `json:"closed,omitempty"`
}

type WalkthroughTerminalCloseRequest struct {
	SessionID string `json:"sessionId"`
}
