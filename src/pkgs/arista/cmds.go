package arista

import (
	"fmt"
	"github.com/montybeatnik/arista-lab/laber/pkgs/renderer"
)

// BGPSummary renders and executes a `show bgp summary` eAPI request.
func (c eosClient) BGPSummary() (BGPEvpnSummaryResponse, error) {
	cmds := []string{"show bgp summary"}
	tmplPath := "templates/eapi_payload.tmpl"
	fmt.Println("rendering template...")
	body, err := renderer.RenderTemplate(tmplPath, renderer.PayloadData{
		Method:  "runCmds",
		Version: 1,
		Format:  "json",
		Cmds:    cmds,
	})
	if err != nil {
		fmt.Printf("failed to render template: %v\n", err)
		return BGPEvpnSummaryResponse{}, fmt.Errorf("failed to render template: %v", err)
	}
	fmt.Println("running cmd...")
	var bgpEvpnSummaryResp BGPEvpnSummaryResponse
	if err := c.Run(body, &bgpEvpnSummaryResp); err != nil {
		fmt.Printf("Run failed: %v\n", err)
		return BGPEvpnSummaryResponse{}, fmt.Errorf("run failed: %v", err)
	}
	return bgpEvpnSummaryResp, nil
}

// Version renders and executes a `show version` eAPI request.
func (c eosClient) Version() (VersionResp, error) {
	cmds := []string{"show version"}
	tmplPath := "templates/eapi_payload.tmpl"
	fmt.Println("rendering template...")
	body, err := renderer.RenderTemplate(tmplPath, renderer.PayloadData{
		Method:  "runCmds",
		Version: 1,
		Format:  "json",
		Cmds:    cmds,
	})
	if err != nil {
		fmt.Printf("failed to render template: %v\n", err)
		return VersionResp{}, fmt.Errorf("failed to render template: %v", err)
	}
	fmt.Println("running cmd...")
	var versionResp VersionResp
	if err := c.Run(body, &versionResp); err != nil {
		fmt.Printf("Run failed: %v\n", err)
		return VersionResp{}, fmt.Errorf("run failed: %v", err)
	}
	return versionResp, nil
}
