# 小小轩桌宠

YHagent 的独立 Windows 桌面客户端。桌宠支持文字、按键语音、气泡、澄清选项和桌面动作。游戏问答使用 YHagent 的流式 OpenAI 兼容门面。

## 安装与连接

运行 `小小轩-Setup-<版本>.exe`。每次安装都会创建「小小轩」桌面快捷方式，Windows 开始菜单和运行时右下角托盘也有入口。更新时选原安装目录，即可沿用已有配置。在设置首页填写 YHagent 服务地址和访问令牌，再保存并连接。客户端支持填写 HTTP 或 HTTPS 地址，并补齐末尾的 `/v1`。HTTP 会以明文传输令牌和对话，公网长期使用建议配置 HTTPS。同机开发服务可以用 `http://127.0.0.1:8080`。

内置战术侦察员随状态眨眼、摆头、迈步和说话。可在「装扮」切换野外侦察和深蓝夜勤。已经在使用侦察员的安装覆盖更新后沿用所选皮肤。

双击桌宠输入文字，或轻按并松开一次左 Alt，马上再次按住左 Alt 说话，松手发送。安装包内置 FunASR SenseVoiceSmall 和 MeloTTS，语音识别和播报均在本机完成。问答需要能够连接已启用 OpenAI 兼容门面的 YHagent 服务。

直播模式可在设置的 World 页面开启。打开 B 站直播间面板填写房间号，然后在「登录 B 站」生成二维码，用手机哔哩哔哩 App 扫码并确认。登录凭证保存在本机，直播间会尝试自动重连。使用 OBS 时可将面板提供的本机 Overlay 地址加入浏览器源。启用后直播事件会进入桌宠会话。

## 构建 Windows x64 安装包

从仓库根目录运行以下命令。首次构建需要访问 npm、GitHub 和模型镜像。

```powershell
git submodule update --init --recursive
cd desktop-pet
corepack pnpm install --frozen-lockfile
corepack pnpm run build:installer
```

生成文件位于 `desktop-pet/dist/小小轩-Setup-<版本>.exe`。开发时可以用 `corepack pnpm run dev`。启动前设置 `YHAGENT_BASE_URL` 可更改首次生成的默认地址。自动化联调可通过进程环境传入 `YHAGENT_TOKEN`。

## 上游与许可证

小小轩客户端基于 Pal AI Lab 的 Coopanion v0.2.23（`0728785f0720fdbd9987f13be25bd9336923e4e4`）改编，使用的 Cortico 子模块固定在 `cba606f6aa9f5d56af033529cfc652fda9918709`。本版于 2026-11-21 更新产品界面。客户端对应源码按 [AGPL-3.0-or-later](LICENSE) 发布，安装后可通过托盘菜单「关于小小轩」查看。语音模型条款随安装包提供。 [FunASR 模型许可](installer/FunASR-MODEL-LICENSE.txt) · [MeloTTS 模型许可](https://huggingface.co/csukuangfj/vits-melo-tts-zh_en/blob/a0d5c6a264c0ef92d70d8661d8cc502d79627cd6/LICENSE)
