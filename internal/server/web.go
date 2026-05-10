package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var webAssets embed.FS

func (s *Server) assets() http.Handler {
	sub, err := fs.Sub(webAssets, "static/assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.ServeFileFS(w, r, webAssets, "static/index.html")
}
