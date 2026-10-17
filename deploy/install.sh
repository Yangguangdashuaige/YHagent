#!/usr/bin/env bash
#
# yhagent-server 首次安装脚本（在 Linux 服务器上以 root 手工执行）
#
# 用法：
#   tar -xzf yhagent-deploy-linux-amd64.tar.gz
#   cd yhagent-deploy
#   sudo bash install.sh            # 初始化目录/用户，自动生成 .env 骨架
#   sudo vi /opt/yhagent/.env       # 填写 ARK_API_KEY / SERVER_TOKEN 等
#   sudo bash install.sh            # 再次运行完成安装并启动
#
# 幂等：可重复执行。已存在的 /opt/yhagent/.env 与 data/ 永远不会被覆盖。
set -euo pipefail

NAME="yhagent"
APP_DIR="/opt/${NAME}"
DATA_DIR="${APP_DIR}/data/kb-extra"
SERVICE_FILE="/etc/systemd/system/${NAME}.service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NEW_BIN="${SCRIPT_DIR}/yhagent-server"
UNIT_SRC="${SCRIPT_DIR}/yhagent.service"
ENV_EXAMPLE="${SCRIPT_DIR}/.env.example"
ENV_FILE="${APP_DIR}/.env"

log()  { printf '\033[1;32m[install]\033[0m %s\n' "$*"; }
info() { printf '\033[1;36m[info]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "请用 root 执行：sudo bash install.sh"
[ -f "$NEW_BIN" ]  || die "找不到二进制 ${NEW_BIN}，请先在 Windows 运行 deploy/build.ps1 并上传部署包"
[ -f "$UNIT_SRC" ] || die "缺少 ${UNIT_SRC}，请确认部署包完整解压"

# ---- 1. 校验二进制确实是 Linux ELF（防止误传 Windows 编译产物） ----
ELF_MAGIC="$(printf '\177ELF')"
if ! head -c 4 "$NEW_BIN" | LC_ALL=C grep -qF -- "$ELF_MAGIC" 2>/dev/null; then
    die "${NEW_BIN} 不是 Linux 可执行文件。请在 Windows 上执行 deploy/build.ps1 重新打包（GOOS=linux）。"
fi

# ---- 2. 创建专用系统用户（无登录权限） ----
if ! id "$NAME" >/dev/null 2>&1; then
    log "创建系统用户 ${NAME}"
    useradd --system --home-dir "$APP_DIR" --shell /usr/sbin/nologin "$NAME"
fi

# ---- 3. 目录结构 ----
log "准备目录 ${APP_DIR}（releases 存历史版本，data 存补充知识库）"
mkdir -p "$APP_DIR/releases" "$DATA_DIR"

# ---- 4. 配置文件：只初始化一次，绝不覆盖线上密钥 ----
if [ ! -f "$ENV_FILE" ]; then
    [ -f "$ENV_EXAMPLE" ] || die "缺少 ${ENV_EXAMPLE}，无法初始化 .env"
    cp "$ENV_EXAMPLE" "$ENV_FILE"
    chmod 600 "$ENV_FILE"
    chown "$NAME:$NAME" "$ENV_FILE"
    cat <<EOF

${NAME}: 已生成配置骨架 ${ENV_FILE}
${NAME}: 请先填写以下必填项后【重新运行本脚本】：
${NAME}:   - ARK_API_KEY / ARK_MODEL_ENDPOINT（火山方舟）
${NAME}:   - DELTA_API_KEY（三角洲开放平台）
${NAME}:   - SERVER_TOKEN（网页访问码，建议 openssl rand -hex 32）
${NAME}:   - ADMIN_TOKEN（可选；联网知识入库口令，留空则关闭入库）
${NAME}:
${NAME}: 生成随机口令：openssl rand -hex 32
${NAME}: 编辑命令：    vi ${ENV_FILE}

EOF
    exit 0
fi

# 非注释行仍含“请替换”占位符，说明还没填完，拒绝启动
if grep -vE '^[[:space:]]*#' "$ENV_FILE" | grep -q '请替换'; then
    die "${ENV_FILE} 中仍有“请替换”占位符未填写，编辑完成后重新运行：vi ${ENV_FILE}"
fi
chown "$NAME:$NAME" "$ENV_FILE"
chmod 600 "$ENV_FILE"

# ---- 5. 安装二进制 ----
log "安装二进制 ${APP_DIR}/yhagent-server"
install -m 0755 "$NEW_BIN" "${APP_DIR}/yhagent-server"

# ---- 6. data 目录归属（补充知识库需要运行时写入） ----
chown -R "$NAME:$NAME" "${APP_DIR}/data"
chmod 750 "${APP_DIR}/data" 2>/dev/null || true

# ---- 7. 安装 systemd unit 并启动 ----
log "安装 systemd 服务 ${SERVICE_FILE}"
install -m 0644 "$UNIT_SRC" "$SERVICE_FILE"
systemctl daemon-reload
systemctl enable "$NAME" >/dev/null 2>&1 || true

log "重启服务 ${NAME}"
systemctl restart "$NAME"
sleep 2

if systemctl is-active --quiet "$NAME"; then
    log "服务运行正常"
    echo
    systemctl --no-pager --full status "$NAME" | head -n 12 || true
    echo
    info "查看实时日志：journalctl -u ${NAME} -f"
    info "补充知识库目录：${DATA_DIR}（采纳的联网知识落盘于此，更新版本不会丢失）"
    if command -v ss >/dev/null 2>&1; then
        LISTEN="$(ss -ltnp 2>/dev/null | grep "${APP_DIR}/yhagent-server" || true)"
        [ -n "$LISTEN" ] && info "监听端口：$LISTEN"
    fi
else
    echo
    systemctl --no-pager --full status "$NAME" | head -n 20 || true
    die "服务启动失败，请查看日志：journalctl -u ${NAME} -n 80 --no-pager"
fi
