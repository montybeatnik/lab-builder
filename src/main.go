package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/montybeatnik/arista-lab/laber/internal/app"
)

func main() {
	cfg := app.DefaultConfig()
	srv, err := app.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	println("listening on", cfg.Listen, "basedir:", cfg.BaseDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
