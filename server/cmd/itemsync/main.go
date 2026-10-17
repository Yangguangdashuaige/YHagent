// cmd/itemsync/main.go
//
// 全物品图鉴同步工具（只在你自己的开发机上运行，不部署到服务器）：
//
//  1. 调用三角洲数据帝【全物品】基础信息库（item_info_all，单次20点资源，
//     2000+ 物品），原始 JSON 缓存到 .cache/item_info_all.json，
//     重复运行默认走缓存不花资源点（-refresh 强制重新拉取）；
//  2. 每件物品渲染成一篇 Markdown 写入 data/knowledge/（文件名"物品-"前缀），
//     静态属性（品质/穿甲/伤害倍率/护甲等级/容量等）入 RAG；
//     实时价格、行情不入库——那些走 deltaapi 在线工具查询。
//  3. 跑完后执行 go run ./cmd/indexer 重建索引，再重新 go build 部署。
//
// 用法（在 server/ 目录下）：
//
//	go run ./cmd/itemsync            # 首次拉取并生成，之后默认用缓存重新生成
//	go run ./cmd/itemsync -refresh   # 赛季更新后重新拉取全物品数据
//	go run ./cmd/itemsync -probe     # 只打印数据统计与样例，不写 Markdown
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"yhagent-server/internal/config"
	"yhagent-server/internal/data/deltaapi"
)

const cacheFile = ".cache/item_info_all.json"

// itemInfo 全物品基础信息库条目（字段以数据帝文档为准）
type itemInfo struct {
	Desc          string          `json:"desc"`         // 文本介绍
	Detail        json.RawMessage `json:"detail"`       // 扩展参数（因类别而异的 JSON 字符串）
	Grade         int             `json:"grade"`        // 品质等级 0~6
	ID            int64           `json:"id"`           // 第二代数据ID（已废弃）
	IsGet         int             `json:"is_get"`       // 是否可在交易行交易 0/1
	Length        int             `json:"length"`       // 占地：长（格）
	ObjectID      int64           `json:"objectID"`     // 三角洲官方小程序ID
	ObjectName    string          `json:"objectName"`   // 名称
	OID           int64           `json:"oid"`          // 第三代数据ID（其他接口用）
	Pic           string          `json:"pic"`          // 图片地址
	PrimaryClass  string          `json:"primaryClass"` // 一级分类
	SecondClass   string          `json:"secondClass"`  // 二级分类（英文/原始值）
	SecondClassCN string          `json:"secondClassCN"`
	Width         int             `json:"width"` // 占地：宽（格）
}

func main() {
	outDir := flag.String("out", "data/knowledge", "Markdown 输出目录（与 indexer 的 -src 一致）")
	refresh := flag.Bool("refresh", false, "忽略本地缓存，重新调用全物品接口（消耗20点资源）")
	probe := flag.Bool("probe", false, "只打印数据统计和样例，不写 Markdown")
	flag.Parse()

	if err := config.Load(); err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	if config.AppConfig.DeltaAPIKey == "" {
		log.Fatal("缺少 DELTA_API_KEY，请确认 server/.env 已配置")
	}

	raw, err := loadRawItems(*refresh)
	if err != nil {
		log.Fatalf("获取全物品数据失败: %v", err)
	}

	var items []itemInfo
	if err := json.Unmarshal(raw, &items); err != nil {
		log.Fatalf("解析物品数据失败: %v", err)
	}
	log.Printf("[itemsync] 共 %d 件物品", len(items))

	if *probe {
		probeStats(items)
		return
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}
	n, err := renderItems(*outDir, items)
	if err != nil {
		log.Fatalf("生成 Markdown 失败: %v", err)
	}
	log.Printf("[itemsync] 完成：写入 %d 篇物品图鉴到 %s（随后请跑 go run ./cmd/indexer）", n, *outDir)
}

// loadRawItems 优先读本地缓存；缓存不存在或 -refresh 时调用接口并回写缓存
func loadRawItems(forceRefresh bool) (json.RawMessage, error) {
	if !forceRefresh {
		if b, err := os.ReadFile(cacheFile); err == nil {
			log.Printf("[itemsync] 使用本地缓存 %s（加 -refresh 可重新拉取）", cacheFile)
			return b, nil
		}
	}

	log.Printf("[itemsync] 调用数据帝全物品基础信息库（消耗20点资源点）...")
	client := deltaapi.NewClient(config.AppConfig.DeltaAPIKey, config.AppConfig.DeltaAPIBase).
		WithHTTPClient(&http.Client{Timeout: 60 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	raw, err := client.AllItemInfo(ctx)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(cacheFile, raw, 0o644); err != nil {
		log.Printf("[itemsync] 警告：缓存写入失败: %v", err)
	}
	return raw, nil
}

// probeStats 打印分类/品质分布与各类别 detail 字段，供核对渲染映射
func probeStats(items []itemInfo) {
	classCount := map[string]int{}
	gradeCount := map[int]int{}
	detailKeys := map[string]map[string]string{} // class -> key -> 样例值
	tradeable := 0
	for _, it := range items {
		classCount[it.PrimaryClass]++
		gradeCount[it.Grade]++
		if it.IsGet == 1 {
			tradeable++
		}
		kv := parseDetail(it.Detail)
		if detailKeys[it.PrimaryClass] == nil {
			detailKeys[it.PrimaryClass] = map[string]string{}
		}
		for k, v := range kv {
			if _, ok := detailKeys[it.PrimaryClass][k]; !ok {
				detailKeys[it.PrimaryClass][k] = fmt.Sprint(v)
			}
		}
	}
	classes := make([]string, 0, len(classCount))
	for c := range classCount {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	fmt.Println("== 一级分类分布 ==")
	for _, c := range classes {
		fmt.Printf("  %-12s %d\n", c, classCount[c])
	}
	fmt.Println("== 品质等级分布 ==")
	for g := 0; g <= 6; g++ {
		if gradeCount[g] > 0 {
			fmt.Printf("  grade%d: %d\n", g, gradeCount[g])
		}
	}
	fmt.Printf("== 可交易: %d / %d ==\n", tradeable, len(items))
	fmt.Println("== 各类别 detail 字段（key=样例值）==")
	for _, c := range classes {
		keys := make([]string, 0, len(detailKeys[c]))
		for k := range detailKeys[c] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("[%s] %s\n", c, strings.Join(keys, ", "))
		for _, k := range keys {
			v := detailKeys[c][k]
			if len([]rune(v)) > 40 {
				v = string([]rune(v)[:40]) + "..."
			}
			fmt.Printf("    %s = %s\n", k, v)
		}
	}
	// 三个完整样例
	fmt.Println("== 完整样例（前3件有 desc 或 detail 的）==")
	shown := 0
	for _, it := range items {
		if it.Desc == "" && len(parseDetail(it.Detail)) == 0 {
			continue
		}
		b, _ := json.MarshalIndent(it, "", "  ")
		fmt.Println(string(b))
		if shown++; shown >= 3 {
			break
		}
	}
}

// parseDetail 解析 detail：接口里它可能是 JSON 对象，也可能是 JSON 字符串
func parseDetail(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		return m
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
		var m2 map[string]any
		if err := json.Unmarshal([]byte(s), &m2); err == nil {
			return m2
		}
	}
	return nil
}
