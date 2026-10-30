// Package embedded 承载知识库主索引的编译期兜底。
//
// 历史设计：index.gob（约几十 MB 的向量库产物）直接 go:embed 进二进制。
// 2026-10-30 起该产物移出 git（每次重建索引都会让 git 历史膨胀几十 MB）：
//   - git 内只保留本 stub.gob 占位，保证任意克隆都能编译；
//   - indexer 产物写到 server/data/index.gob，部署包随 data/ 分发；
//   - 服务端启动时优先读 data/index.gob，读不到才回退本包的嵌入内容
//     （stub 为空 → DecodeGOB 失败 → 知识库能力关闭并记日志，见 cmd/server）。
//
// 更新流程：编辑 knowledge/*.md → go run ./cmd/indexer（产出 data/index.gob）
// → 重新打包部署（deploy/build.ps1 会把 data/index.gob 打进部署包）。
package embedded

import _ "embed"

// IndexGOB 编译期嵌入的兜底索引。正常部署走 data/index.gob 文件加载，
// 此处 embed 的 stub.gob 为 0 字节占位——含义是"无嵌入库，必须随包携带数据文件"。
//
//go:embed stub.gob
var IndexGOB []byte
