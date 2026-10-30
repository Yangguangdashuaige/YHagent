<div align="center">

# YHagent · 三角洲行动 AI 助手

**小小轩** —— 基于豆包大模型 Function Call 的《三角洲行动》游戏助手
实时物价 · 战备配装 · 改枪推荐 · 截图识别 · 赛季任务 · 地图密码

<br />

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Wails](https://img.shields.io/badge/Wails-v2-2980a8)
![CGO](https://img.shields.io/badge/CGO-zero-success)
![Arch](https://img.shields.io/badge/Deploy-Linux%20amd64%20%7C%20Windows-555)
![Status](https://img.shields.io/badge/status-%E5%BC%80%E5%8F%91%E4%B8%AD-orange)

</div>

---

## ✨ 功能一览

| 模块 | 能力 |
| --- | --- |
|智能对话 | 多轮上下文 + Function Call 自主决策工具，SSE 流式逐字输出 |
|实时数据 | 接入三角洲数据帝开放平台 **24 个工具**：物品价格、假账排行、卡战备、钥匙卡、倒子弹、制造/兑换利润、赛季任务、今日密码、市集物品等 |
|智能改枪顾问 | 简称自动补全枪名（`K437` → `K437突击步枪`）、聚合最多 96 个社区方案、解析八维属性按「压枪 / 腰射 / 稳定 / 操控 / 射程 / 均衡 / 省钱 / 热度」排序，输出 Top5 可直接导入的改枪码 |
|截图识别（二期） | 网页版支持选择 / 粘贴游戏截图，多模态识别改枪界面、仓库、价格页，先确认识别结果再给建议 |
|双端形态 | Wails 桌面悬浮窗（Windows EXE）+ 浏览器网页版，共用同一套 SSE 接口 |
|静态知识库 | 内置 Markdown 切片 + 向量检索 RAG 框架（1970 篇源文档随仓库分发、克隆即得），支持热更新知识库内容 |
|工程化 | 全用户共享缓存（按上游更新频率分级 TTL + 5000 条 LRU 上限）、singleflight 防击穿、会话空闲自动清理、Bearer 令牌鉴权、零 CGO 单二进制部署 |

## 整体架构

```text
┌──────────────────────┐        ┌──────────────────────┐
│  Wails 桌面客户端     │        │  浏览器网页版         │
│  (client / EXE)      │        │  (server/web/dist)   │
│  悬浮窗 · 打字机效果  │        │  附图 · 粘贴截图      │
└──────────┬───────────┘        └──────────┬───────────┘
           │   HTTPS + Bearer Token        │
           └───────────────┬───────────────┘
                           │  SSE（chunk/done/error）
                           ▼
              ┌────────────────────────┐
              │   yhagent-server       │  纯 net/http，零 CGO
              │  Agent 多轮工具循环     │
              │  24 工具 / 会话管理     │
              │  分级 TTL 缓存 + LRU   │
              │  静态知识库 RAG        │
              └─────┬────────────┬─────┘
                    │            │
                    ▼            ▼
          ┌──────────────┐ ┌────────────────────┐
          │ 火山方舟豆包  │ │ 三角洲数据帝开放API │
          │ 对话/视觉模型 │ │ 物价·改枪码·任务等  │
          └──────────────┘ └────────────────────┘
```

**单仓双 Module**，根目录 `go.work` 串联，客户端不依赖任何服务端代码，两边仅靠 SSE 事件协议对话：

- `client/`（module `yhagent-client`）：Wails 瘦客户端，不持有任何 API Key，只负责界面与事件转发
- `server/`（module `yhagent-server`）：全部智能能力、密钥、缓存、知识库都收敛在服务端，十万客户端共享一份缓存

## 目录结构

```text
YHagent/
├── go.work                     # 串联 client / server 两个 module
├── client/                     # Wails 桌面瘦客户端
│   ├── app.go                  # 前端绑定层（事件协议：chat:chunk/done/error）
│   ├── internal/
│   │   ├── config/             # .env 加载
│   │   └── remote/             # SSE 远端对话客户端
│   └── frontend/dist/          # 悬浮窗界面（原生 HTML/CSS/JS）
└── server/                     # 云服务器（module yhagent-server）
    ├── cmd/
    │   ├── server/             # HTTP/SSE 服务入口
    │   └── indexer/            # 知识库离线建库工具
    ├── internal/
    │   ├── ai/                 # Agent 核心：多轮 Function Call、会话、工具注册
    │   ├── data/
    │   │   ├── cache/          # TTL + LRU 内存缓存
    │   │   └── deltaapi/       # 三角洲 API 客户端 + 24 个工具 + 智能改枪顾问
    │   ├── knowledge/          # 文档切片 / 向量检索 / search_knowledge 工具
    │   └── server/             # HTTP 路由、鉴权、SSE、图片校验
    ├── data/
    │   ├── knowledge/          # 静态知识源（Markdown，下划线开头文件跳过，不入库）
    │   └── kb-extra/           # 运行时补充知识库（联网核实后管理员确认入库）
    └── web/dist/               # 网页版（go:embed 编入二进制）
```

## 快速开始

### 前置要求

- Go ≥ 1.26
- 桌面端开发需要 [Wails CLI](https://wails.io/zh-Hans/docs/gettingstarted/installation)
- 火山方舟（豆包）API Key 与接入点 ID（需支持视觉输入的对话模型）
- [三角洲数据帝开放平台](https://orzice.com/workApi) API Key

### 1. 启动服务端

```bash
cd server
cp .env.example .env       # 填入真实密钥
go run ./cmd/server
```

启动成功后：

- API 与网页版同在 `:8080`，浏览器打开 <http://127.0.0.1:8080/>，输入 `SERVER_TOKEN` 作为访问码进入网页版
- 看到日志 `已注册 24 个工具` 即正常；知识库为空时 `search_knowledge` 自动不注册，不影响对话

### 2. 启动桌面客户端

```bash
cd client
cp .env.example .env       # REMOTE_BASE_URL / REMOTE_TOKEN 指向服务端
wails dev                  # 开发调试
wails build                # 产出 Windows EXE
```

### 3. 构建静态知识库索引（可选但推荐）

知识库源文档（1970 篇：物品图鉴 / 官方公告 / 机制详解 / 方法论）**已随仓库分发**，克隆即得。
首次运行 indexer 生成向量索引 `data/index.gob`（产物不进 git，57MB）：

```bash
cd server
go run ./cmd/indexer      # 增量建库：全量嵌入一次约几千次调用；此后只对新增/修改切片重嵌
go run ./cmd/server       # 重启后自动注册 search_knowledge 工具
```

> ⚠️ Embedding 模型（默认 `doubao-embedding-vision-251215`）需先在[火山方舟控制台](https://console.volcengine.com/ark/region:ark+cn-beijing/openManagement)开通。
> 不跑 indexer 也能启动：知识库检索不可用，其余功能不受影响。
> `data/compat/gunparts.json`（改枪求解器数据）同理可随时用 `go run ./cmd/partssync` 重新生成。

### 4. 同步 B 站视频到知识库（可选）

`cmd/bilisync` 把游戏教学视频转成带时间戳锚点的 Markdown 知识文档：登录态拉取 B 站 AI 字幕（`ai-zh`，秒级时间轴）与官方 AI 大纲 → 碎片字幕按停顿合并自然段、按官方大纲分大节 → 豆包去口头语、修正 ASR 错字并按干员/流言主题切 `###` 小标题 → 写入 `data/knowledge/视频-{BV号}-{标题}.md`，再跑 indexer 增量入库。

```bash
cd server
# .env 中配置 BILI_SESSDATA（浏览器登录B站后的 Cookie，AI字幕仅登录态可见）
go run ./cmd/bilisync BV1kBfVYhEAs            # 支持视频链接，多P自动全量
go run ./cmd/bilisync BVxxxx -raw             # 只分段不调豆包，零 API 费用
go run ./cmd/indexer                          # 增量向量化新文档
```

- 正文中的 `[mm:ss]` 对应原片时间点，引用时可直接定位视频
- 思考型模型首字延迟可能达数分钟，属正常现象；单节失败自动降级为原始分段稿，不阻断同步
- 无任何中文字幕轨的视频需改走 ASR 路线（抽音频 + 录音文件识别），当前版本未包含

### 5. 联网知识回流（可选，维护者工作流）

线上用户采纳的联网知识会实时存进服务器的 `data/kb-extra/`（运行时补充索引）。要让这些内容**进入 git 与后续所有部署包**（而不是散落在服务器上），定期回流：

```bash
# 1. 从服务器拉回线上补充库
scp -r root@server:/opt/yhagent/data/kb-extra ./kb-extra-from-server

# 2. 迁移到静态知识库（同名幂等，可重复执行）
cd server
go run ./cmd/kbmerge -src ../kb-extra-from-server

# 3. 版本化 + 增量建库 + 出新部署包
git add data/knowledge && git commit
go run ./cmd/indexer
# 之后 deploy/build.ps1 → 服务器 update.sh；
# 服务器上按 kbmerge 输出的提示清理旧补充库，避免同一内容双份命中
```

## 环境变量

### 服务端 `server/.env`

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `ARK_API_KEY` | 是 | 火山方舟 API Key |
| `ARK_MODEL_ENDPOINT` | 是 | 对话模型接入点 ID（如 `ep-xxxxxxxx`），需支持视觉输入 |
| `ARK_BASE_URL` | 否 | 默认 `https://ark.cn-beijing.volces.com/api/v3` |
| `ARK_EMBEDDING_MODEL` | 否 | 知识库向量化模型，默认 `doubao-embedding-vision-251215` |
| `DELTA_API_KEY` | 是 | 三角洲数据帝开放平台 Key |
| `DELTA_API_BASE` | 否 | 默认 `https://orzice.com/workApi` |
| `SERVER_ADDR` | 否 | 监听地址，默认 `:8080` |
| `SERVER_TOKEN` | 是 | 客户端 / 网页版访问令牌，建议 `openssl rand -hex 32`，留空拒绝启动 |
| `BILI_SESSDATA` | 否 | B 站 Cookie，仅 `cmd/bilisync` 同步视频时需要，AI 字幕必须登录态 |

### 客户端 `client/.env`

| 变量 | 说明 |
| --- | --- |
| `REMOTE_BASE_URL` | 服务端地址，正式环境请使用 HTTPS |
| `REMOTE_TOKEN` | 与服务端 `SERVER_TOKEN` 一致 |

## 📡 HTTP / SSE 协议

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/chat` | 发起对话，SSE 流式返回；请求体 `{"session_id","message","images"}`。同会话忙时 SSE 推送 error；满员返回 503，超频返回 429 |
| `POST` | `/v1/session/new` | 服务端签发会话 ID（`sess_` + 128bit 随机），返回 `{"session_id"}`；旧版 UUID v4 格式过渡期仍兼容 |
| `POST` | `/v1/session/reset` | 清空指定会话上下文；问答进行中返回 409 |
| `GET` | `/v1/auth/check` | 校验令牌有效性（网页版登录用） |
| `GET` | `/healthz` | 健康检查 |

入口保护：`session_id` 必须为服务端签发格式或历史 UUID v4，非法 ID 在 SSE 建连前返回 `400 {"code":"invalid_session"}`（前端自动换发 ID 重试）；对话类接口按 IP 每分钟限流（默认 30 次），`/v1/chat` 全局并发闸门默认 16（可用 `CHAT_RPM_PER_IP` / `CHAT_MAX_CONCURRENT` 调整）。

SSE 事件：`event: chunk`（JSON 编码文本片段）、`event: done`、`event: error`，每 25 秒发送 `: ping` 心跳。

截图消息约束：`images` 为 data URL 数组，**最多 3 张、单张 ≤ 4MB、请求体 ≤ 12MB**，支持 JPEG / PNG / WebP / GIF；网页端会自动压缩到 1280px 的 JPEG。

## 🐧 Linux 部署（交叉编译）

在 Windows 开发机上一键交叉编译零依赖单二进制并打部署包（已验证产物为 ELF x86-64）：

```powershell
.\deploy\build.ps1               # 默认 linux/amd64；ARM 服务器加 -Arch arm64
```

产物 `deploy/yhagent-deploy-linux-amd64.tar.gz`（约 27MB，含二进制、systemd unit、`install.sh`、`update.sh`、`.env.example`），SHA256 会打印在末尾。

**首次部署**（上传到服务器后执行）：

```bash
scp deploy/yhagent-deploy-linux-amd64.tar.gz root@<服务器IP>:/root/
ssh root@<服务器IP>
tar -xzf yhagent-deploy-linux-amd64.tar.gz
cd yhagent-deploy
sudo bash install.sh            # ① 创建 yhagent 用户、/opt/yhagent 目录，生成 .env 骨架后退出
sudo vi /opt/yhagent/.env       # ② 填写 ARK_API_KEY / ARK_MODEL_ENDPOINT / DELTA_API_KEY / SERVER_TOKEN
                                #    ADMIN_TOKEN 选填：联网知识入库口令（openssl rand -hex 32 生成）
sudo bash install.sh            # ③ 再次运行：安装 systemd 服务并启动，自动健康检查
```

**后续更新**（只换二进制，`.env` 与 `data/kb-extra` 补充知识库原样保留；新版本启动失败自动回滚上一版，旧版保留最近 5 个）：

```powershell
# Windows 上重新打包
.\deploy\build.ps1
```

```bash
# 服务器上解压新包后
cd yhagent-deploy && sudo bash update.sh
```

脚本约定：部署根目录 `/opt/yhagent`（二进制、`.env`、`releases/` 历史版本、`data/kb-extra/` 采纳的联网知识），systemd 服务名 `yhagent`，已配置 `WorkingDirectory`、`Restart=always`、`LimitNOFILE=65535` 与基础安全加固。常用运维命令：`systemctl status yhagent`、`journalctl -u yhagent -f`。

Nginx 反向代理时设置 `proxy_buffering off;` 并调大 `client_max_body_size 16m;`（SSE 流式 + 截图请求最大 12MB）。如服务器开启了 firewalld/安全组，放行 SERVER_ADDR 对应端口（默认 8080）。

## 技术栈

- **语言 / 运行时**：Go（标准库 `net/http` + SSE，零 CGO，可交叉编译单二进制）
- **AI**：火山方舟豆包（对话 + 视觉 + Embedding）、ark-runtime-go
- **桌面端**：Wails v2（WebView2 悬浮窗）
- **网页端**：原生 HTML / CSS / JavaScript，fetch + ReadableStream 手写 SSE 解析，`go:embed` 编入二进制
- **检索**：本地余弦相似度 TopK（`float32` 向量 + gob 持久化），无外部数据库依赖
- **稳定性**：singleflight 请求合并、分级 TTL 缓存、LRU 容量保险丝、panic 恢复、超时降级

## 说明

- 所有游戏数据来自三角洲数据帝开放平台社区与公开接口，**数据仅供参考**；改枪码为社区玩家方案，实际手感因人而异
- 缓存 TTL 严格按上游文档标注的更新频率分级：基础信息库 24h、改枪码 / 赛季任务 6h、每日更新类 2h、每分钟更新的行情类 60s
- 会话空闲 2 小时自动清理（10 分钟扫描一次），日活万级单实例内存即可承载；多实例 / 持久化（Redis 等）留待下一阶段
