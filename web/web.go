// Package web holds the embedded static frontend assets.
package web

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
