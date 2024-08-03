package tmpl

import (
	"embed"
	"mime"
	"path"
	"text/template"
	"time"
)

//go:embed templates
var tmplFS embed.FS
var tmpl *template.Template

func init() {
	tmpl = template.New("")
	tmpl = tmpl.Funcs(template.FuncMap{
		"add":    func(a int, b int) int { return a + b },
		"datef":  func(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) },
		"datef2": func(t time.Time) string { return t.UTC().Format(time.DateOnly) },
		"base":   func(s string) string { return path.Base(s) },
		"ext":    func(s string) string { return path.Ext(s) },
		"mime":   func(s string) string { return mime.TypeByExtension(path.Ext(s)) },
	})
	println(mime.TypeByExtension(".opf"))
	println(mime.TypeByExtension(".ncx"))
	tmpl = template.Must(tmpl.ParseFS(tmplFS, "templates/*"))
}
