# 小小轩桌宠

YHagent 的独立 Windows 桌面客户端。桌宠支持文字、按键语音、气泡、澄清选项和桌面动作。游戏问答使用 YHagent 的流式 OpenAI 兼容门面。

## 安装与连接

运行 `YHagent-Pet-Setup-0.1.0.exe`。启动后从托盘打开设置，在首页填写 YHagent 服务地址和访问令牌，再保存并连接。远程地址使用 HTTPS，客户端会补齐末尾的 `/v1`。同机开发服务可以用 `http://127.0.0.1:8080`。

双击桌宠输入文字，或按住左 Alt 说话后松手发送。安装包内置 FunASR SenseVoiceSmall，语音识别在本机完成。问答需要能够连接已启用 OpenAI 兼容门面的 YHagent 服务。

直播模式可在设置的 World 页面开启。打开 B 站直播间面板，填写房间号，按需要登录，并把面板的本机 Overlay 地址加入 OBS 浏览器源。启用后直播事件会进入桌宠会话。

## 构建 Windows x64 安装包

从仓库根目录运行以下命令。首次构建需要访问 npm、GitHub 和模型镜像。

```powershell
git submodule update --init --recursive
cd desktop-pet
corepack pnpm install --frozen-lockfile
corepack pnpm run build:installer
```

生成文件位于 `desktop-pet/dist/YHagent-Pet-Setup-0.1.0.exe`。开发时可以用 `corepack pnpm run dev`。启动前设置 `YHAGENT_BASE_URL` 可更改首次生成的默认地址。自动化联调可通过进程环境传入 `YHAGENT_TOKEN`。

## 上游与许可证

客户端提取自 Coopanion v0.2.23（`0728785f0720fdbd9987f13be25bd9336923e4e4`）。Cortico 子模块固定在 `cba606f6aa9f5d56af033529cfc652fda9918709`。客户端对应源码按 [AGPL-3.0-or-later](LICENSE) 发布。模型条款随安装包提供，开发树中的副本见 [FunASR 模型许可](installer/FunASR-MODEL-LICENSE.txt)。
