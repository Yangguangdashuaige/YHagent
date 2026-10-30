#!/usr/bin/env bash
#
# yhagent-server 版本更新脚本（在 Linux 服务器上以 root 手工执行）
#
# 用法：
#   tar -xzf yhagent-deploy-linux-amd64.tar.gz
#   cd yhagent-deploy
#   sudo bash update.sh
#
# 行为：
#   - 只替换二进制；/opt/yhagent/.env 与 data/（补充知识库）原样保留
#   - 旧版本先备份到 /opt/yhagent/releases/，新版本启动失败自动回滚
set -euo pipefail

NAME="yhagent"
APP_DIR="/opt/${NAME}"
CUR_BIN="${APP_DIR}/yhagent-server"
SERVICE_FILE="/etc/systemd/system/${NAME}.service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NEW_BIN="${SCRIPT_DIR}/yhagent-server"
TS="$(date +%Y%m%d-%H%M%S)"
BAK_BIN="${APP_DIR}/releases/yhagent-server-${TS}"

log()  { printf '\033[1;32m[update]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "请用 root 执行：sudo bash update.sh"
[ -f "$NEW_BIN" ] || die "找不到新二进制 ${NEW_BIN}，请先解压新部署包并进入 yhagent-deploy 目录"
[ -f "$CUR_BIN" ] || die "当前未安装 ${CUR_BIN}，首次部署请改用：sudo bash install.sh"
[ -f "$SERVICE_FILE" ] || die "未找到 ${SERVICE_FILE}，首次部署请改用：sudo bash install.sh"

# ---- 1. 校验新产物是 Linux ELF ----
ELF_MAGIC="$(printf '\177ELF')"
if ! head -c 4 "$NEW_BIN" | LC_ALL=C grep -qF -- "$ELF_MAGIC" 2>/dev/null; then
    die "${NEW_BIN} 不是 Linux 可执行文件，请勿上传 Windows 产物。"
fi

# ---- 2. 展示版本差异（文件大小/校验和） ----
log "当前版本：$(stat -c '%s 字节  %y' "$CUR_BIN" 2>/dev/null || stat -f '%z 字节' "$CUR_BIN")"
log "新版本：  $(stat -c '%s 字节  %y' "$NEW_BIN" 2>/dev/null || stat -f '%z 字节' "$NEW_BIN")"
OLD_SUM="$(sha256sum "$CUR_BIN" | awk '{print $1}')"
NEW_SUM="$(sha256sum "$NEW_BIN" | awk '{print $1}')"
if [ "$OLD_SUM" = "$NEW_SUM" ]; then
    die "新旧二进制 SHA256 完全相同，无需更新（${NEW_SUM:0:16}...）。"
fi

# ---- 3. 备份当前版本 ----
mkdir -p "${APP_DIR}/releases"
cp -a "$CUR_BIN" "$BAK_BIN"
log "已备份旧版本到 ${BAK_BIN}"

# ---- 4. 替换并重启 ----
log "安装新二进制"
install -m 0755 "$NEW_BIN" "$CUR_BIN"

# 随包数据文件一并更新（知识库索引/改枪配件库）；
# kb-extra（玩家采纳的联网知识）等运行时用户数据不在部署包里，不受影响
for rel in 'index.gob' 'compat/gunparts.json'; do
    src="${SCRIPT_DIR}/data/${rel}"
    if [ -f "$src" ]; then
        dst="${APP_DIR}/data/${rel}"
        mkdir -p "$(dirname "$dst")"
        install -m 0644 "$src" "$dst"
        log "已更新数据文件 data/${rel}"
    fi
done

# unit 文件若包内有更新则一并刷新（ yhagent.service 与脚本同目录 ）
if [ -f "${SCRIPT_DIR}/yhagent.service" ]; then
    if ! cmp -s "${SCRIPT_DIR}/yhagent.service" "$SERVICE_FILE"; then
        log "systemd unit 有变化，已更新"
        install -m 0644 "${SCRIPT_DIR}/yhagent.service" "$SERVICE_FILE"
        systemctl daemon-reload
    fi
fi

log "重启服务"
systemctl restart "$NAME"
sleep 2

# ---- 5. 健康检查，失败自动回滚 ----
if systemctl is-active --quiet "$NAME"; then
    log "更新成功，服务运行中"
    echo
    journalctl -u "$NAME" -n 15 --no-pager || true
    echo
    log "如需手工回滚："
    echo "  sudo install -m 0755 ${BAK_BIN} ${CUR_BIN} && sudo systemctl restart ${NAME}"
    # 只保留最近 5 个历史版本
    # shellcheck disable=SC2012
    ls -1t "${APP_DIR}/releases"/yhagent-server-* 2>/dev/null | tail -n +6 | while read -r old; do
        rm -f "$old"
        warn "已清理过期备份：$old"
    done
    exit 0
fi

warn "新版本启动失败，自动回滚到 ${BAK_BIN}"
install -m 0755 "$BAK_BIN" "$CUR_BIN"
systemctl restart "$NAME" || true
sleep 2
if systemctl is-active --quiet "$NAME"; then
    warn "已回滚到旧版本，服务恢复正常。新版本仍保留在 ${NEW_BIN}"
else
    warn "回滚后服务仍异常，请立即人工排查：journalctl -u ${NAME} -n 80 --no-pager"
fi
journalctl -u "$NAME" -n 40 --no-pager || true
die "更新失败，已回滚。"
