<#
.SYNOPSIS
    在 Windows 开发机上交叉编译 yhagent-server（零 CGO 单二进制）并打包，
    供拷贝到 Linux 服务器后由 install.sh / update.sh 人工部署。

.EXAMPLE
    .\deploy\build.ps1
    编译 linux/amd64 部署包（绝大多数 x86 云服务器）。

.EXAMPLE
    .\deploy\build.ps1 -Arch arm64
    编译 linux/arm64 部署包（鲲鹏 / 树莓派 / AWS Graviton 等）。

.NOTES
    产物：deploy/yhagent-deploy-linux-<arch>.tar.gz
    内容：yhagent-server、yhagent.service、install.sh、update.sh、.env.example
    部署见 README「Linux 部署」一节。
#>
[CmdletBinding()]
param(
    [ValidateSet('amd64', 'arm64')]
    [string]$Arch = 'amd64',

    # 跳过 go vet（默认编译前先 vet）
    [switch]$SkipVet
)

$ErrorActionPreference = 'Stop'

$deployDir = $PSScriptRoot
$repoRoot  = Split-Path -Parent $deployDir
$serverDir = Join-Path $repoRoot 'server'
$stage     = Join-Path $deployDir 'yhagent-deploy'
$outTar    = Join-Path $deployDir "yhagent-deploy-linux-$Arch.tar.gz"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "未找到 go 命令，请先安装 Go 1.26+ 并加入 PATH。"
}
if (-not (Test-Path (Join-Path $serverDir 'go.mod'))) {
    throw "未找到 $serverDir\go.mod，请从仓库根目录运行：.\deploy\build.ps1"
}

Write-Host "==> 准备打包目录 $stage" -ForegroundColor Cyan
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Force $stage | Out-Null

Write-Host "==> 交叉编译 linux/$Arch（CGO_ENABLED=0）" -ForegroundColor Cyan
Push-Location $serverDir
try {
    if (-not $SkipVet) {
        Write-Host "==> go vet ./..." -ForegroundColor DarkGray
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'go vet 失败' }
    }
    $env:CGO_ENABLED = '0'
    $env:GOOS        = 'linux'
    $env:GOARCH      = $Arch
    $binPath = Join-Path $stage 'yhagent-server'
    go build -trimpath -ldflags '-s -w' -o $binPath ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'go build 失败' }
}
finally {
    Pop-Location
    # 恢复本机环境，避免影响后续本地 go run / 测试
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Host '==> 收集部署文件' -ForegroundColor Cyan
Copy-Item (Join-Path $deployDir 'yhagent.service') $stage
Copy-Item (Join-Path $deployDir 'install.sh')      $stage
Copy-Item (Join-Path $deployDir 'update.sh')       $stage
Copy-Item (Join-Path $serverDir '.env.example')    $stage

# 关键：Windows 检出可能是 CRLF，bash 执行 CRLF 脚本会报 '\r' 错误，统一转 LF（无 BOM）
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
foreach ($name in 'install.sh', 'update.sh') {
    $p = Join-Path $stage $name
    $text = [System.IO.File]::ReadAllText($p) -replace "`r`n", "`n"
    [System.IO.File]::WriteAllText($p, $text, $utf8NoBom)
}

Write-Host "==> 打包 $outTar" -ForegroundColor Cyan
if (Test-Path $outTar) { Remove-Item $outTar -Force }
# Windows 10 1803+ 自带 bsdtar；老系统可装 7-Zip 后自行压缩 yhagent-deploy 目录
& tar -C $deployDir -czf $outTar yhagent-deploy
if ($LASTEXITCODE -ne 0) { throw 'tar 打包失败（需 Windows 10 1803+ 自带 tar，或安装 7-Zip）' }

$hash = (Get-FileHash $outTar -Algorithm SHA256).Hash
$size = [math]::Round((Get-Item $outTar).Length / 1MB, 2)
Write-Host ''
Write-Host '部署包已生成：' -ForegroundColor Green
Write-Host "  $outTar ($size MB)"
Write-Host "  SHA256: $hash"
Write-Host ''
Write-Host '下一步（上传到服务器后）：' -ForegroundColor Yellow
Write-Host "  tar -xzf yhagent-deploy-linux-$Arch.tar.gz"
Write-Host '  cd yhagent-deploy && sudo bash install.sh   # 首次安装'
Write-Host '  sudo bash update.sh                          # 后续更新（不碰 .env / data）'
