package handler

import (
	"html/template"
	"net/http"

	"github.com/logerror/t2t/pkg/config"

	"github.com/logerror/t2t/internal/server/web"
)

func ServeIndexPage(w http.ResponseWriter, r *http.Request) {
	// 如果不是根路径，返回 404
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tmpl := template.Must(template.ParseFS(web.TemplateFiles, "templates/index.html"))
	err := tmpl.Execute(w, nil)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func IndexHelper(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.Redirect(w, r, config.Configuration.HelpUrl, http.StatusFound)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
