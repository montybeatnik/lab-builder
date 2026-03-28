package app

import (
	"io"
	"net/http"
	"time"
)

type Handlers struct {
	cfg       Config
	templates TemplateExecutor
}

type TemplateExecutor interface {
	ExecuteTemplate(wr io.Writer, name string, data any) error
}

func NewHandlers(cfg Config, templates TemplateExecutor) *Handlers {
	return &Handlers{cfg: cfg, templates: templates}
}

func NewMux(h *Handlers) (*http.ServeMux, error) {
	mux := http.NewServeMux()
	staticDir, err := resolveAssetPath("web/static")
	if err != nil {
		return nil, err
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.HandleFunc("/", h.Index)
	mux.HandleFunc("/lab", h.Lab)
	mux.HandleFunc("/viewer", h.Viewer)
	mux.HandleFunc("/live", h.Live)
	mux.HandleFunc("/walkthroughs", h.Walkthroughs)
	mux.HandleFunc("/labs", h.Labs)
	mux.HandleFunc("/labplan", h.LabPlan)
	mux.HandleFunc("/lab/nodes", h.LabNodes)
	mux.HandleFunc("/lab/config", h.LabNodeConfig)
	mux.HandleFunc("/inspect", h.Inspect)
	mux.HandleFunc("/runcmd", h.RunCmd)
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/topology/validate", h.TopologyValidate)
	mux.HandleFunc("/topology/render-config", h.TopologyRenderConfig)
	mux.HandleFunc("/topology/build", h.TopologyBuild)
	mux.HandleFunc("/topology/deploy", h.TopologyDeploy)
	mux.HandleFunc("/topology/destroy", h.TopologyDestroy)
	mux.HandleFunc("/topology/delete", h.TopologyDelete)
	mux.HandleFunc("/topology/live", h.TopologyLive)
	mux.HandleFunc("/topology/traffic", h.TopologyTraffic)
	mux.HandleFunc("/walkthroughs/catalog", h.WalkthroughCatalog)
	mux.HandleFunc("/walkthroughs/preflight", h.WalkthroughPreflight)
	mux.HandleFunc("/walkthroughs/launch", h.WalkthroughLaunch)
	mux.HandleFunc("/walkthroughs/terminal", h.WalkthroughTerminal)
	mux.HandleFunc("/walkthroughs/terminal/start", h.WalkthroughTerminalStart)
	mux.HandleFunc("/walkthroughs/terminal/write", h.WalkthroughTerminalWrite)
	mux.HandleFunc("/walkthroughs/terminal/poll", h.WalkthroughTerminalPoll)
	mux.HandleFunc("/walkthroughs/terminal/close", h.WalkthroughTerminalClose)
	mux.HandleFunc("/walkthroughs/terminal/ws", h.WalkthroughTerminalWS)
	return mux, nil
}

func NewServer(cfg Config) (*http.Server, error) {
	templates, err := NewTemplates()
	if err != nil {
		return nil, err
	}
	mux, err := NewMux(NewHandlers(cfg, templates))
	if err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
}
