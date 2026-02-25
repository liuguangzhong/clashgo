# ────────────────────────────────────────────────────────
# ClashGo 一键启动脚本 (开发模式, Windows PowerShell)
# Usage: .\scripts\dev.ps1
# ────────────────────────────────────────────────────────
$ErrorActionPreference = "Stop"

function Info { param([string]$msg) Write-Host "[INFO]  $msg" -ForegroundColor Cyan }
function Ok { param([string]$msg) Write-Host "[OK]    $msg" -ForegroundColor Green }
function Fail { param([string]$msg) Write-Host "[FAIL]  $msg" -ForegroundColor Red; exit 1 }

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = (Resolve-Path "$ScriptDir\..").Path
Set-Location $ProjectDir

# 环境检查
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Fail "Go 未安装" }
if (-not (Get-Command node -ErrorAction SilentlyContinue)) { Fail "Node.js 未安装" }
if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) { Fail "pnpm 未安装。请先运行: .\scripts\setup.ps1" }

if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    Info "安装 Wails CLI..."
    & go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
    $env:PATH += ";$(go env GOPATH)\bin"
    if (-not (Get-Command wails -ErrorAction SilentlyContinue)) { Fail "Wails CLI 安装失败" }
}

# 确保前端依赖
if (-not (Test-Path "frontend\node_modules")) {
    Info "安装前端依赖..."
    Push-Location frontend
    & pnpm install
    Pop-Location
}

Ok "环境就绪，启动开发模式..."
Write-Host ""
Write-Host "  [package] 前端热更新: Vite HMR" -ForegroundColor Magenta
Write-Host "  [wrench]  后端热更新: Wails Auto-rebuild" -ForegroundColor Magenta
Write-Host "  [globe]   按 Ctrl+C 停止" -ForegroundColor Magenta
Write-Host ""

& wails dev
