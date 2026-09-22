// Package embedded 通过 go:embed 把离线生成的知识库索引 index.gob
// 编进服务端二进制，部署时无需携带任何知识库数据文件。
//
// 更新流程：编辑 knowledge/*.md → 运行 cmd/indexer 重新生成 index.gob
// → 重新 go build / 部署即可。
package embedded

import _ "embed"

// IndexGOB 是 cmd/indexer 生成的 gob 格式知识库索引
//
//go:embed index.gob
var IndexGOB []byte
