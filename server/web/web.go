// web/web.go
//
// 服务端静态资源：编译期通过 go:embed 打进服务端二进制，
// 部署到云服务器时仍然只有一个可执行文件。
// 聊天客户端（Wails 桌面端与网页聊天页）已随阶段 35 桌宠化改造退役，
// 当前仅保留 trace.html 观测页；桌面侧由 Coopanion 经 OpenAI 兼容门面接入。
package web

import "embed"

// Dist 静态资源根目录（dist/trace.html 观测页）
//
//go:embed all:dist
var Dist embed.FS
