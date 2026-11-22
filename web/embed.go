// Package web embeds the browser client served to display devices and the
// host page shown on the host computer.
package web

import "embed"

// Files holds the display client (index.html, app.js, style.css) and the
// host page (host.html, host.js, host.css).
//
//go:embed index.html app.js style.css host.html host.js host.css
var Files embed.FS
