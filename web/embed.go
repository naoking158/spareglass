// Package web embeds the browser client served to display devices.
package web

import "embed"

// Files holds index.html, app.js and style.css.
//
//go:embed index.html app.js style.css
var Files embed.FS
