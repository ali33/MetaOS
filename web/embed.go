// Package web nhúng giao diện đã build (web/dist) vào metaos-ws.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist là gốc của web/dist.
var Dist, _ = fs.Sub(dist, "dist")
