// Package docs embeds the Markdown documentation that is served by the API
// at /docs. Keeping the files in the repository means the web documentation
// and the files on GitHub never diverge.
package docs

import "embed"

// FS contains every Markdown document and the Postman collection.
//
//go:embed ru/*.md api/*.json
var FS embed.FS
