package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var staticFiles embed.FS

// ServeStatic returns an http.Handler that serves the React SPA.
func ServeStatic() http.Handler {
	raw, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		panic("web: could not read embedded index.html: " + err.Error())
	}
	indexHTML := raw

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("web: could not sub embedded static FS: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)

		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			fileServer.ServeHTTP(w, r)
			return
		}

		if p != "/" && p != "/index.html" {
			f, err := sub.Open(strings.TrimPrefix(p, "/"))
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexHTML)
	})
}
