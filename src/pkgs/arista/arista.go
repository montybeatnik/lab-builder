package arista

// BGPEvpnSummaryResponse mirrors EOS JSON-RPC output for `show bgp summary`.
type BGPEvpnSummaryResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Result  []BGPEvpnSummaryResult `json:"result"`
}

// BGPEvpnSummaryResult groups BGP summary data by VRF name.
type BGPEvpnSummaryResult struct {
	Vrfs map[string]VRF `json:"vrfs"`
}

// VRF holds router metadata and peers keyed by neighbor address.
type VRF struct {
	VRF      string          `json:"vrf"`
	RouterID string          `json:"routerId"`
	ASN      string          `json:"asn"`
	Peers    map[string]Peer `json:"peers"`
}

// Peer is one BGP neighbor snapshot as reported by EOS.
type Peer struct {
	Version          int     `json:"version"`
	MsgReceived      int     `json:"msgReceived"`
	MsgSent          int     `json:"msgSent"`
	InMsgQueue       int     `json:"inMsgQueue"`
	OutMsgQueue      int     `json:"outMsgQueue"`
	ASN              string  `json:"asn"`
	PrefixAccepted   int     `json:"prefixAccepted"`
	PrefixReceived   int     `json:"prefixReceived"`
	UpDownTime       float64 `json:"upDownTime"`
	UnderMaintenance bool    `json:"underMaintenance"`
	PeerState        string  `json:"peerState"`
	PrefixAdvertised int     `json:"prefixAdvertised"`
}

// VersionResp mirrors EOS JSON-RPC output for `show version`.
type VersionResp struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  []VersionDetails `json:"result"`
}

// VersionDetails carries device inventory/build fields returned by EOS.
type VersionDetails struct {
	MfgName            string  `json:"mfgName"`
	ModelName          string  `json:"modelName"`
	HardwareRevision   string  `json:"hardwareRevision"`
	SerialNumber       string  `json:"serialNumber"`
	SystemMacAddress   string  `json:"systemMacAddress"`
	HwMacAddress       string  `json:"hwMacAddress"`
	ConfigMacAddress   string  `json:"configMacAddress"`
	Version            string  `json:"version"`
	Architecture       string  `json:"architecture"`
	InternalVersion    string  `json:"internalVersion"`
	InternalBuildID    string  `json:"internalBuildId"`
	ImageFormatVersion string  `json:"imageFormatVersion"`
	ImageOptimization  string  `json:"imageOptimization"`
	KernelVersion      string  `json:"kernelVersion"`
	BootupTimestamp    float64 `json:"bootupTimestamp"`
	Uptime             float64 `json:"uptime"`
	MemTotal           int     `json:"memTotal"`
	MemFree            int     `json:"memFree"`
	IsIntlVersion      bool    `json:"isIntlVersion"`
}
