<div align="center">

# YHagent · 三角洲行动 AI 助手

**小小轩** —— 基于豆包大模型 Function Call 的《三角洲行动》游戏助手
实时物价 · 战备配装 · 改枪推荐 · 截图识别 · 赛季任务 · 地图密码

<br />

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Coopanion](https://img.shields.io/badge/Pal--AI--Lab-AGPL--3.0-8f2f2f)
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
|桌宠形态 | 基于 [Coopanion](https://github.com/Pal-AI-Lab/Coopanion)，支持屏幕底边走动、气泡回答、语音输入和装扮，经 OpenAI 兼容门面接入（阶段 35 起 Wails 客户端与网页聊天页退役） |
|截图识别 | 截屏经 Coopanion 以多模态消息上传，先确认识别结果再给建议 |
|静态知识库 | 内置 Markdown 切片 + 向量检索 RAG 框架（1967 篇源文档随仓库分发、克隆即得），支持热更新知识库内容 |
|工程化 | 全用户共享缓存（按上游更新频率分级 TTL + 5000 条 LRU 上限）、singleflight 防击穿、会话空闲自动清理、Bearer 令牌鉴权、零 CGO 单二进制部署 |

## 整体架构

```text
┌──────────────────────────────┐
│  Coopanion 桌宠客户端         │
│  (Electron · 沿屏幕底边走动)   │
│  气泡 · 按键说话 · 本地ASR     │
│  pet_* 客户端工具             │
└──────────────┬───────────────┘
               │ HTTPS + Bearer Token
               │ OpenAI Chat Completions 兼容协议
               │ （/v1/chat/completions 门面）
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

**单 Module 纯服务端**（阶段 35 起 Wails 客户端与网页聊天页退役，桌宠为唯一桌面形态）：

- 桌面端是 [Coopanion](https://github.com/Pal-AI-Lab/Coopanion)（原版发行版 + 自定义端点配置，零客户端代码），经 OpenAI 兼容门面把 YHagent 当作"超级模型端点"
- `server/`（module `yhagent-server`）：全部智能能力、密钥、缓存、知识库都收敛在服务端，客户端不持有任何业务 API Key

## 目录结构

```text
YHagent/
├── server/                     # 云服务器（module yhagent-server）
    ├── cmd/
    │   ├── server/             # HTTP/SSE 服务入口
    │   ├── indexer/            # 知识库离线建库工具
    │   └── mcpserver/          # MCP stdio 服务独立入口
    ├── internal/
    │   ├── ai/                 # Agent 核心：多轮 Function Call、会话、工具注册
    │   ├── data/
    │   │   ├── cache/          # TTL + LRU 内存缓存
    │   │   └── deltaapi/       # 三角洲 API 客户端 + 24 个工具 + 智能改枪顾问
    │   ├── knowledge/          # 文档切片 / 向量检索 / search_knowledge 工具
    │   ├── openaicompat/       # OpenAI 兼容门面（Coopanion 桌宠通道）
    │   └── server/             # HTTP 路由、鉴权、SSE、图片校验
    ├── data/
    │   ├── knowledge/          # 静态知识源（Markdown，下划线开头文件跳过，不入库）
    │   └── kb-extra/           # 运行时补充知识库（联网核实后管理员确认入库）
    └── web/dist/               # trace.html 观测页（go:embed 编入二进制）
```

## 快速开始

### 前置要求

- Go ≥ 1.26
- 火山方舟（豆包）API Key 与接入点 ID（需支持视觉输入的对话模型）
- [三角洲数据帝开放平台](https://orzice.com/workApi) API Key
- 桌面端安装**小小轩桌宠**（YHagent-Pet，基于 Coopanion 的预配置发行版）

### 1. 启动服务端

```bash
cd server
cp .env.example .env       # 填入真实密钥；桌宠通道设 OPENAICOMPAT_ENABLED=1
go run ./cmd/server
```

启动成功后：

- 看到日志 `OpenAI 兼容门面已启用：/v1/chat/completions` 与 `已注册 24 个工具` 即正常；知识库为空时 `search_knowledge` 自动不注册，不影响对话

### 2. 安装小小轩桌宠

Windows 客户端在 [desktop-pet](desktop-pet/README.md)。安装后在首页连接 YHagent，双击桌宠打字，或轻按一次左 Alt 后立即再次按住说话。
安装包内置 FunASR 语音识别。游戏数据工具由服务端执行，气泡、动作与提问选项在客户端执行。

### 3. 构建静态知识库索引（可选但推荐）

知识库源文档（1967 篇：物品图鉴 / 官方公告 / 机制详解 / 视频）**已随仓库分发**，克隆即得。
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
| `MCP_TOKEN` | 否 | MCP 服务器令牌，配置后主服务挂 `/mcp` 路由（见「MCP 服务器」一节） |
| `MCP_EXCLUDE` | 否 | MCP 不暴露的工具名单（逗号分隔），默认 `web_search,plan_and_execute` |
| `OPENAICOMPAT_ENABLED` | 否 | =1 启用 OpenAI 兼容门面（Coopanion 桌宠通道），默认关闭 |
| `OPENAICOMPAT_MODEL_NAME` | 否 | 门面对外模型名，默认 `yhagent-delta` |
| `OPENAICOMPAT_MAX_TOOL_ROUNDS` | 否 | 门面循环单回合工具轮数上限，默认 8 |
| `OPENAICOMPAT_PENDING_TTL_MIN` | 否 | 影子回合成活期（分钟，等待客户端工具回执），默认 10 |

### 客户端（Coopanion）

客户端零配置文件——在 Coopanion 的「模型供应商」里添加自定义端点即可（API 地址 + `SERVER_TOKEN`），详见「快速开始 · 2」。

## 📡 HTTP / SSE 协议

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/v1/chat/completions` | **OpenAI 兼容门面**（`OPENAICOMPAT_ENABLED=1` 启用）：Coopanion/任意 OpenAI 客户端以此接入。请求携带全量 messages + 客户端工具 schema；响应为标准 chunk 流（文本/思考流/tool_calls/finish/usage） |
| `GET` | `/v1/models` | 门面模型列表（单模型，Cortico 设置页下拉用） |
| `POST` | `/v1/chat` | 发起对话，SSE 流式返回；请求体 `{"session_id","message","images"}`。同会话忙时 SSE 推送 error；满员返回 503，超频返回 429 |
| `POST` | `/v1/session/new` | 服务端签发会话 ID（`sess_` + 128bit 随机），返回 `{"session_id"}`；旧版 UUID v4 格式过渡期仍兼容 |
| `POST` | `/v1/session/reset` | 清空指定会话上下文；问答进行中返回 409 |
| `GET` | `/v1/auth/check` | 校验令牌有效性 |
| `GET` | `/healthz` | 健康检查 |

入口保护：`session_id` 必须为服务端签发格式或历史 UUID v4，非法 ID 在 SSE 建连前返回 `400 {"code":"invalid_session"}`（前端自动换发 ID 重试）；对话类接口按 IP 每分钟限流（默认 30 次），`/v1/chat` 全局并发闸门默认 128——满员不拒绝，新请求先到先得排队（可用 `CHAT_RPM_PER_IP` / `CHAT_MAX_CONCURRENT` / `CHAT_QUEUE_TIMEOUT` 调整）；门面路由同样叠加鉴权与按 IP 限流。

SSE 事件：`event: chunk`（JSON 编码文本片段）、`event: queue`（并发满员时的排队位置，`{"waiting":N}`，0=已入场）、`event: done`、`event: error`，每 25 秒发送 `: ping` 心跳。

截图消息约束：`images` 为 data URL 数组，**最多 3 张、单张 ≤ 4MB、请求体 ≤ 12MB**，支持 JPEG / PNG / WebP / GIF；网页端会自动压缩到 1280px 的 JPEG。

## 🔌 MCP 服务器（把工具集开放给任意 MCP 客户端）

YHagent 的工具集（`search_knowledge` 知识库检索、23 个三角洲数据工具、`gun_combo_solve` 改枪求解器）可整体暴露为标准 MCP 服务器，Claude Desktop / Trae / 其他 Agent 直接挂载使用，工具代码零改动（`internal/mcp` 桥接 + `internal/bootstrap` 共用装配）。两种形态：

**形态一：远程 Streamable HTTP**（推荐，多人共享服务端数据与缓存）

`.env` 配置 `MCP_TOKEN`（长随机串，独立于 SERVER_TOKEN）后主服务自动挂 `/mcp` 路由（带鉴权与按 IP 限流）。客户端配置：

```json
{
  "mcpServers": {
    "yhagent": {
      "url": "https://你的域名/mcp",
      "headers": { "Authorization": "Bearer <MCP_TOKEN>" }
    }
  }
}
```

**形态二：本地 stdio**（部署包内 `yhagent-mcp` 二进制，客户端以子进程拉起）

```json
{
  "mcpServers": {
    "yhagent": {
      "command": "/opt/yhagent/yhagent-mcp",
      "env": { "ARK_API_KEY": "ark-xxx", "DELTA_API_KEY": "xxx" }
    }
  }
}
```

stdio 形态要求工作目录能访问 `data/`（`index.gob`、`kb-extra/`、`compat/gunparts.json`），密钥走进程环境变量；Windows 本地开发用 `go run ./cmd/mcpserver`。

**工具白名单**：`MCP_EXCLUDE` 控制不暴露的工具（逗号分隔），默认排除 `web_search`（消耗方舟联网配额，客户端普遍自带联网）与 `plan_and_execute`（依赖本 Agent 会话循环）。每次工具调用会消耗上游 API（embedding / 三角洲数据），凭据务必用长随机串。

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
