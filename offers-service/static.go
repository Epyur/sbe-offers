package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// Страница Swagger UI и схема OpenAPI (правило проекта: сервис сам показывает,
// что умеет). Дистрибутив UI лежит в репозитории, без CDN — сервис работает
// без внешней сети. Лицензия Apache-2.0 (static/swagger/LICENSE).
//
// Пути обязательно внутри /api/offers/: Caddy проксирует сервису только этот
// префикс, поэтому корневой /docs/ снаружи не откроется.

//go:embed static/swagger
var swaggerUIFS embed.FS

var swaggerFS, _ = fs.Sub(swaggerUIFS, "static/swagger")

//go:embed static/openapi.yaml
var openapiYAML []byte

func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiYAML)
}
