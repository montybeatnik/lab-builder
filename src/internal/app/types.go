package app

import (
	"encoding/json"

	"github.com/montybeatnik/arista-lab/laber/labplanner"
	"github.com/montybeatnik/arista-lab/laber/labstore"
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

type HealthReq struct {
	Lab        string `json:"lab"`
	UseSudo    bool   `json:"sudo"`
	TimeoutSec int    `json:"timeoutSec"`
	User       string `json:"user"`
	Pass       string `json:"pass"`
}

type HealthCheck struct {
	Name   string `json:"name"`
	Result string `json:"result"`
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

type LabsResponse struct {
	OK    bool                 `json:"ok"`
	Error string               `json:"error,omitempty"`
	Labs  []labstore.LabRecord `json:"labs,omitempty"`
}

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
}

type ProtocolSetJSON struct {
	Global []string            `json:"global"`
	Roles  map[string][]string `json:"roles"`
}

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

type MonitoringConfig struct {
	SNMP bool `json:"snmp"`
	GNMI bool `json:"gnmi"`
}

type Check struct {
	Name   string `json:"name"`
	Result string `json:"result"`
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

type DestroyRequest struct {
	LabName string `json:"labName"`
	UseSudo bool   `json:"sudo"`
}

type RenderConfigRequest struct {
	TopologyRequest
	NodeName string `json:"nodeName"`
}

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

type LiveTopologyRequest struct {
	LabName string `json:"labName"`
	UseSudo bool   `json:"sudo"`
}

type LiveEndpointStatus struct {
	Node  string `json:"node"`
	Iface string `json:"iface"`
	State string `json:"state"`
}

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

type LiveTopologyResponse struct {
	OK       bool             `json:"ok"`
	Error    string           `json:"error,omitempty"`
	LabName  string           `json:"labName,omitempty"`
	Nodes    []NodePlanJSON   `json:"nodes,omitempty"`
	Links    []LiveLinkStatus `json:"links,omitempty"`
	Summary  LiveSummary      `json:"summary,omitempty"`
	PolledAt string           `json:"polledAt,omitempty"`
}
