package app

import (
	"html/template"
	"os"
	"path/filepath"
)

func NewTemplates() (*template.Template, error) {
	root, err := resolveAssetPath("web/templates")
	if err != nil {
		return nil, err
	}

	return template.ParseFS(
		os.DirFS("."),
		filepath.Join(root, "layout.tmpl"),
		filepath.Join(root, "partials.tmpl"),
		filepath.Join(root, "index.tmpl"),
		filepath.Join(root, "lab.tmpl"),
		filepath.Join(root, "viewer.tmpl"),
		filepath.Join(root, "live.tmpl"),
		filepath.Join(root, "walkthroughs.tmpl"),
	)
}
