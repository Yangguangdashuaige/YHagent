// web/web.go
//
// 网页版入口的静态资源：编译期通过 go:embed 打进服务端二进制，
// 因此部署到云服务器时仍然只有一个可执行文件，不需要额外携带前端文件。
// 网页版与桌面客户端共用同一套 /v1/* 接口与 SSE 事件协议。
package web

import "embed"

// Dist 网页版静态资源根目录（dist/index.html、style.css、app.js）
//
//go:embed all:dist
var Dist embed.FS
