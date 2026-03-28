package renderer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"text/template"
	// "github.com/montybeatnik/arista-lab/laber/pkgs/arista"
)

// PayloadData captures the JSON-RPC envelope fields expected by EOS eAPI templates.
type PayloadData struct {
	Method  string
	Version int
	Format  string
	Cmds    []string
	ID      int
}

// RenderTemplate renders a named template file and returns its serialized request body bytes.
func RenderTemplate(tplPath string, data PayloadData) ([]byte, error) {
	funcs := template.FuncMap{
		"toJSON": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
	}
	base := filepath.Base(tplPath)
	tpl, err := template.New(base).Funcs(funcs).ParseFiles(tplPath)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, base, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
