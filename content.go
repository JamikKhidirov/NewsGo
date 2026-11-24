// Package gonews embeds top-level repository documents so that the running
// binary can serve them without depending on the working directory.
package gonews

import "embed"

// Content contains the project README, license and environment template.
//
//go:embed README.md LICENSE all:.env.example
var Content embed.FS
