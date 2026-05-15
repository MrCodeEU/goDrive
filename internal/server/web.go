package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var webAssets embed.FS

func (s *Server) assets() http.Handler {
	sub, err := fs.Sub(webAssets, "static/assets")
	if err != nil {
		panic(err)
	}
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if p != "/" && p != "/files" && !strings.HasPrefix(p, "/files/") {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, webAssets, "static/index.html")
}
