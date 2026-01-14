package main

import (
	"net/http"

	"github.com/montybeatnik/arista-lab/laber/labstore"
)

type LabPlanResponse struct {
	OK        bool                      `json:"ok"`
	Error     string                    `json:"error,omitempty"`
	Nodes     []NodePlanJSON            `json:"nodes,omitempty"`
	Links     []LinkAssignedJSON        `json:"links,omitempty"`
	Protocols ProtocolSetJSON           `json:"protocols,omitempty"`
}

type NodePlanJSON struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	ASN       int    `json:"asn"`
	Loopback  string `json:"loopback"`
	EdgeIP    string `json:"edgeIp"`
	EdgePrefix int   `json:"edgePrefix"`
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

func labPlanHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			writeJSON(w, http.StatusBadRequest, LabPlanResponse{OK: false, Error: "name is required"})
			return
		}
		db, err := labstore.OpenLabDB(cfg.BaseDir)
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
				Name:      n.Name,
				Role:      n.Role,
				ASN:       n.ASN,
				Loopback:  n.Loopback,
				EdgeIP:    n.EdgeIP,
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
}
