// cmd/kbmerge/main.go
//
// 联网知识回流工具：把线上服务器"运行时采纳的联网知识"（data/kb-extra/）
// 合并回静态知识库源文档（data/knowledge/），使其随 git 版本化、
// 进入下一次 indexer 建库与部署包——而不是永远散落在各服务器上。
//
// 背景与数据流：
//
//	线上 web_search 采纳 → 服务器 data/kb-extra/docs/*.md（ExtraStore，运行时可写）
//	                         │
//	          scp 拉回开发机 ┘
//	  本工具：kb-extra/docs/*.md ──移入──▶ data/knowledge/（git 版本化）
//	  之后：  go run ./cmd/indexer     （增量：只有新文档需要嵌入花费）
//	          deploy/build.ps1          （新 index.gob 进部署包）
//	          服务器 sudo bash update.sh（主索引更新，kb-extra 不受影响）
//	          服务器清理旧补充库（见运行结束打印的提示命令）
//
// 用法（在 server/ 目录下）：
//
//	go run ./cmd/kbmerge -src ../kb-extra-from-server
//
//	-src 指向从服务器拉下来的 kb-extra 目录（内含 docs/*.md 与 extra.gob）。
//	行为：docs/*.md 移入 data/knowledge/（同名已存在则跳过，幂等可重跑）；
//	删除 src 下已迁移的 md 与 extra.gob（旧补充索引作废——内容已进主库）。
//	-dst 可覆盖目标知识目录（默认 data/knowledge）。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	src := flag.String("src", "", "服务器拉回的 kb-extra 目录（必填，内含 docs/*.md）")
	dst := flag.String("dst", "data/knowledge", "静态知识库源文档目录")
	flag.Parse()

	if strings.TrimSpace(*src) == "" {
		log.Fatal("用法：go run ./cmd/kbmerge -src <服务器拉回的kb-extra目录>\n" +
			"  示例：scp root@server:/opt/yhagent/data/kb-extra ./kb-extra-from-server\n" +
			"        go run ./cmd/kbmerge -src ./kb-extra-from-server")
	}

	docsDir := filepath.Join(*src, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		log.Fatalf("读取 %s 失败（目录结构应为 {src}/docs/*.md）: %v", docsDir, err)
	}

	if err := os.MkdirAll(*dst, 0o755); err != nil {
		log.Fatalf("创建目标目录 %s 失败: %v", *dst, err)
	}

	var moved, skipped []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		name := e.Name()
		srcPath := filepath.Join(docsDir, name)
		dstPath := filepath.Join(*dst, name)

		if _, err := os.Stat(dstPath); err == nil {
			// 主库已有同名文档（可能是上次已回流过，或文档名恰好冲突）：跳过并保留源文件
			skipped = append(skipped, name)
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			log.Fatalf("读取 %s 失败: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			log.Fatalf("写入 %s 失败: %v", dstPath, err)
		}
		if err := os.Remove(srcPath); err != nil {
			log.Fatalf("移除源文件 %s 失败（目标已写入，请手工删除避免重复回流）: %v", srcPath, err)
		}
		moved = append(moved, name)
	}

	// 旧补充索引作废：内容已迁入主库源文档，保留会导致回传后同一内容双份命中
	oldIdx := filepath.Join(*src, "extra.gob")
	idxRemoved := false
	if _, err := os.Stat(oldIdx); err == nil {
		if err := os.Remove(oldIdx); err != nil {
			log.Printf("[kbmerge] 警告：删除 %s 失败（可手工删除）: %v", oldIdx, err)
		} else {
			idxRemoved = true
		}
	}

	sort.Strings(moved)
	for _, m := range moved {
		fmt.Printf("  迁入 %s\n", m)
	}
	for _, s := range skipped {
		fmt.Printf("  跳过 %s（主库已存在同名文档）\n", s)
	}

	fmt.Printf("\n迁移 %d 篇 / 跳过 %d 篇；补充索引 %s\n", len(moved), len(skipped), doneStr(idxRemoved))

	if len(moved) > 0 {
		fmt.Println("\n后续步骤：")
		fmt.Println("  1. git add data/knowledge && git commit   # 知识文档版本化")
		fmt.Println("  2. go run ./cmd/indexer                   # 增量建库（新文档嵌入）")
		fmt.Println("  3. deploy/build.ps1 打包 → 服务器 update.sh")
		fmt.Println("  4. 服务器上清理旧补充库（避免同一内容双份命中）：")
		fmt.Println("     rm -f /opt/yhagent/data/kb-extra/extra.gob /opt/yhagent/data/kb-extra/docs/*.md")
		fmt.Println("     systemctl restart yhagent")
	} else if idxRemoved {
		fmt.Println("\n没有新文档，但已清理源目录的过期补充索引。")
	}
}

func doneStr(ok bool) string {
	if ok {
		return "已删除（内容已进主库）"
	}
	return "不存在或删除失败"
}
