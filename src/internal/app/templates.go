package app

import (
	"html/template"
	"os"
)

func NewTemplates() (*template.Template, error) {
	return template.ParseFS(
		os.DirFS("."),
		"web/templates/layout.tmpl",
		"web/templates/index.tmpl",
		"web/templates/lab.tmpl",
		"web/templates/viewer.tmpl",
		"web/templates/live.tmpl",
		"web/templates/walkthroughs.tmpl",
	)
}
