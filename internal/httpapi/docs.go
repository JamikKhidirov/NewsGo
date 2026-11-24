package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openapiSpec []byte

// swaggerHTML is fully self-contained: Swagger UI assets are embedded, so the
// reference works without internet access and inside an air-gapped container.
const swaggerHTML = `<!doctype html>
<html lang="ru" data-theme="light">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light dark">
  <title>GoNews API — Swagger UI</title>
  <link rel="stylesheet" href="/docs/assets/swagger-ui.css">
  <link rel="stylesheet" href="/docs/assets/common.css">
  <link rel="stylesheet" href="/docs/assets/swagger-theme.css">
  <script>try{var t=localStorage.getItem('gonews-theme');document.documentElement.setAttribute('data-theme',t||(window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'));}catch(e){}</script>
</head>
<body class="swagger-page">
<header class="docs-header">
  <a class="docs-brand" href="/docs">
    <span class="docs-logo" aria-hidden="true">G</span>
    <span class="docs-brand-text">GoNews <span class="docs-badge">API</span></span>
  </a>
  <nav class="docs-topnav" aria-label="Основные ссылки">
    <a href="/docs">Документация</a>
    <a href="/docs/openapi.yaml">OpenAPI</a>
    <a href="/docs/postman_collection.json">Postman</a>
    <button type="button" class="theme-toggle" data-theme-toggle aria-label="Переключить тему" title="Переключить тему">
      <span class="theme-icon" aria-hidden="true">🌙</span><span class="theme-label">Тёмная</span>
    </button>
  </nav>
</header>
<main class="swagger-wrap">
  <div id="swagger-ui"></div>
</main>
<script src="/docs/assets/swagger-ui-bundle.js"></script>
<script src="/docs/assets/theme.js"></script>
<script>
  window.addEventListener('load', function () {
    var host = document.getElementById('swagger-ui');
    if (!window.SwaggerUIBundle) {
      host.innerHTML = '<p class="swagger-error">Не удалось загрузить Swagger UI. Откройте спецификацию напрямую: <a href="/docs/openapi.yaml">/docs/openapi.yaml</a>.</p>';
      return;
    }
    SwaggerUIBundle({
      url: '/docs/openapi.yaml',
      dom_id: '#swagger-ui',
      deepLinking: true,
      persistAuthorization: true,
      displayRequestDuration: true,
      filter: true,
      tryItOutEnabled: true,
      presets: [SwaggerUIBundle.presets.apis]
    });
  });
</script>
</body>
</html>`

func (a *API) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(swaggerHTML))
}

func (a *API) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(openapiSpec)
}
