// Package web 前端静态资源,构建产物经 embed.FS 内嵌(§7.1,内网无 CDN)
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
