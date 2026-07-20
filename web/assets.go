// Package web embeds the vendored frontend assets (HTMX, Alpine.js,
// Cytoscape.js, theme CSS) into the binary so production deployment stays a
// single binary plus a Vault directory, with no separate web/ directory or
// CDN dependency at runtime (see ADR-0007).
package web

import "embed"

//go:embed vendor themes js fonts
var FS embed.FS
