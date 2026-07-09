// Package site carries the static status-page assets, embedded into the binary
// so the daemon can publish them to the gh-pages branch without checking out
// the source tree.
package site

import "embed"

// Assets holds the page's static files (index.html, app.js, style.css). The
// generated data/status.json is NOT embedded — the daemon writes it at publish
// time.
//
//go:embed index.html app.js style.css
var Assets embed.FS

// Files lists the embedded asset names to copy when publishing.
var Files = []string{"index.html", "app.js", "style.css"}
