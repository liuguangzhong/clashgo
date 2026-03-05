# ─────────────────────────────────────────────────────────────────────────────
# ClashGo 一键编译脚本 (Windows PowerShell)
# 用法:
#   .\scripts\build.ps1                      # 编译 Windows amd64（当前平台）
#   .\scripts\build.ps1 -All                 # 编译全部平台
#   .\scripts\build.ps1 -Platform linux/amd64
#   .\scripts\build.ps1 -GoOnly              # 仅 go build（无前端，快速验证）
#   .\scripts\build.ps1 -Deps                # 仅更新依赖
#
# 支持的 -Platform 值:
#   windows/amd64  linux/amd64  linux/arm64  darwin/amd64  darwin/arm64
# ─────────────────────────────────────────────────────────────────────────────
param(
    [string]$Platform = "",
    [switch]$All = $false,
    [switch]$GoOnly = $false,
    [switch]$Deps = $false,
    [switch]$Help = $false
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = (Resolve-Path "$ScriptDir\..").Path
Set-Location $ProjectDir

# ── 颜色输出 ───────────────────────────────────────────────────────────────────
function info { param($m) Write-Host "  $m" -ForegroundColor Cyan }
function ok { param($m) Write-Host "✓ $m" -ForegroundColor Green }
function warn { param($m) Write-Host "! $m" -ForegroundColor Yellow }
function fail { param($m) Write-Host "✗ $m" -ForegroundColor Red; exit 1 }
function title { param($m) Write-Host "`n══ $m" -ForegroundColor Magenta }

if ($Help) {
    @"
ClashGo 一键编译脚本
------------------------------------------------------------
用法: .\scripts\build.ps1 [选项]

选项:
  -Platform <平台>  目标平台 (windows/amd64 | linux/amd64 |
                             linux/arm64 | darwin/amd64 | darwin/arm64)
  -All              编译全部平台
  -GoOnly           仅 go build（不含前端，快速验证编译）
  -Deps             只下载/更新依赖（go mod download + tidy）
  -Help             显示此帮助

示例:
  .\scripts\build.ps1
  .\scripts\build.ps1 -Platform linux/amd64
  .\scripts\build.ps1 -All
  .\scripts\build.ps1 -GoOnly
"@
    exit 0
}

# ── 版本信息 ──────────────────────────────────────────────────────────────────
$VERSION = & git describe --tags --always --dirty 2>$null
if (-not $VERSION) { $VERSION = "v1.0.0-dev" }
$BUILD_TIME = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ" -AsUTC 2>$null) ?? (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
$LDFLAGS = "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -s -w"

title "ClashGo $VERSION ($BUILD_TIME)"

# ── 仅更新依赖 ────────────────────────────────────────────────────────────────
if ($Deps) {
    title "更新依赖"
    info "go mod download..."
    & go mod download
    info "go mod tidy..."
    & go mod tidy
    ok "依赖就绪"
    exit 0
}

# ── 环境检查 ──────────────────────────────────────────────────────────────────
title "环境检查"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    fail "Go 未安装，请访问 https://go.dev/dl/ 安装后重试"
}
ok "Go: $(go version)"

if (-not $GoOnly) {
    if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
        warn "Wails 未安装，正在自动安装..."
        & go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
        if ($LASTEXITCODE -ne 0) { fail "Wails 安装失败，请手动运行: go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2" }
        ok "Wails 已安装"
    }
    else {
        ok "Wails: $(wails version 2>$null)"
    }

    if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
        fail "Node.js 未安装，请访问 https://nodejs.org 安装"
    }
    ok "Node.js: $(node --version)"

    # 安装前端依赖
    if (Test-Path "frontend\package.json") {
        $pm = if (Get-Command pnpm -ErrorAction SilentlyContinue) { "pnpm" } else { "npm" }
        info "安装前端依赖 ($pm install)..."
        & $pm install --prefix frontend
        if ($LASTEXITCODE -ne 0) { fail "前端依赖安装失败" }
        ok "前端依赖安装完成"
    }
}

# Go 依赖
info "go mod download..."
& go mod download
& go mod tidy
ok "Go 依赖就绪"

# ── 构建函数 ──────────────────────────────────────────────────────────────────
New-Item -ItemType Directory -Force -Path "build\bin" | Out-Null

function Build-Target {
    param([string]$Plat)

    $parts = $Plat -split "/"
    $goos = $parts[0]
    $goarch = $parts[1]
    $outName = "clashgo-$goos-$goarch"
    if ($goos -eq "windows") { $outName += ".exe" }

    title "构建 $Plat → $outName"

    if ($GoOnly) {
        $env:GOOS = $goos
        $env:GOARCH = $goarch
        & go build -ldflags $LDFLAGS -o "build\bin\$outName" .
        Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    }
    else {
        & wails build -platform $Plat -ldflags $LDFLAGS -o $outName
    }

    if ($LASTEXITCODE -ne 0) { fail "构建 $Plat 失败 (exit=$LASTEXITCODE)" }
    ok "$Plat 构建完成 → build\bin\$outName"
}

# ── 执行构建 ──────────────────────────────────────────────────────────────────
if ($All) {
    $targets = @(
        "windows/amd64",
        "linux/amd64",
        "linux/arm64",
        "darwin/amd64",
        "darwin/arm64"
    )
    foreach ($t in $targets) { Build-Target $t }

}
elseif ($Platform -ne "") {
    Build-Target $Platform

}
else {
    # 默认：Windows amd64
    if ($GoOnly) {
        title "Go only 编译"
        & go build -ldflags $LDFLAGS ./...
        if ($LASTEXITCODE -ne 0) { fail "go build 失败" }
    }
    else {
        title "构建 Windows amd64"
        & wails build -ldflags $LDFLAGS
        if ($LASTEXITCODE -ne 0) { fail "wails build 失败" }
    }
}

# ── 展示结果 ──────────────────────────────────────────────────────────────────
title "构建产物"
if (Test-Path "build\bin") {
    Get-ChildItem "build\bin" -File |
    Sort-Object LastWriteTime -Descending |
    Format-Table Name,
    @{N = "大小(MB)"; E = { [math]::Round($_.Length / 1MB, 2) } },
    LastWriteTime -AutoSize
}

Write-Host ""
ok "══════════════════════════════════════════"
ok "  ClashGo 构建完成！版本: $VERSION"
ok "  输出目录: build\bin\"
ok "══════════════════════════════════════════"
