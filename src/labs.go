package main

import (
	"net/http"

	"github.com/montybeatnik/arista-lab/laber/labstore"
)

type LabsResponse struct {
	OK    bool                 `json:"ok"`
	Error string               `json:"error,omitempty"`
	Labs  []labstore.LabRecord `json:"labs,omitempty"`
}

func labsHandler(cfg serverCfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		db, err := labstore.OpenLabDB(cfg.BaseDir)
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
}
