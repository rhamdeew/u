package web

import (
	"embed"
	"html/template"

	"u/internal/admin"
)

//go:embed templates/* static/*
var FS embed.FS

// ParseTemplates parses all page template sets with the given FuncMap.
func ParseTemplates() (*admin.Templates, error) {
	fm := admin.TemplateFuncMap()

	parse := func(pages ...string) (*template.Template, error) {
		files := append([]string{"templates/base.html"}, pages...)
		return template.New("").Funcs(fm).ParseFS(FS, files...)
	}

	login, err := parse("templates/login.html")
	if err != nil {
		return nil, err
	}
	adminTmpl, err := parse("templates/admin.html")
	if err != nil {
		return nil, err
	}
	edit, err := parse("templates/edit.html")
	if err != nil {
		return nil, err
	}
	stats, err := parse("templates/stats.html")
	if err != nil {
		return nil, err
	}

	return &admin.Templates{
		Login: login,
		Admin: adminTmpl,
		Edit:  edit,
		Stats: stats,
	}, nil
}
