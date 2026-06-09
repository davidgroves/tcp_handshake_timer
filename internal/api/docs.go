package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

//go:embed static/swagger-ui/*
var swaggerUI embed.FS

const docsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>thtimer API</title>
  <link rel="stylesheet" href="/api/docs/static/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="/api/docs/static/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      SwaggerUIBundle({
        url: "/api/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
        layout: "BaseLayout"
      });
    };
  </script>
</body>
</html>`

func (s *Server) swaggerUIHandler() http.Handler {
	sub, err := fs.Sub(swaggerUI, "static/swagger-ui")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/api/docs/static/", http.FileServer(http.FS(sub)))
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(openAPISpec)
}

func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsHTML))
}
