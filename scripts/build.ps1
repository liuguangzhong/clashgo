# ────────────────────────────────────────────────────────
# ClashGo 一键编译脚本 (Windows PowerShell)
# Usage: .\scripts\build.ps1 [-Platform "windows/amd64"]
# ────────────────────────────────────────────────────────
param(
    [string]$Platform = ""
)

$ErrorActionPreference = "Stop"

function Info { param([string]$msg) Write-Host "[INFO]  $msg" -ForegroundColor Cyan }
function Ok { param([string]$msg) Write-Host "[OK]    $msg" -ForegroundColor Green }
function Warn { param([string]$msg) Write-Host "[WARN]  $msg" -ForegroundColor Yellow }
function Fail { param([string]$msg) Write-Host "[FAIL]  $msg" -ForegroundColor Red; exit 1 }

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = (Resolve-Path "$ScriptDir\..").Path
Set-Location $ProjectDir

# ── 1. 环境检查 ────────────────────────────────────────
Info "检查编译环境..."

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { Fail "Go 未安装。请访问 https://go.dev/dl/" }
if (-not (Get-Command node -ErrorAction SilentlyContinue)) { Fail "Node.js 未安装。请访问 https://nodejs.org/" }
if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) { Fail "pnpm 未安装。请先运行: .\scripts\setup.ps1" }

if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    Warn "Wails CLI 未安装，正在安装..."
    & go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
    $env:PATH += ";$(go env GOPATH)\bin"
    if (-not (Get-Command wails -ErrorAction SilentlyContinue)) { Fail "Wails CLI 安装失败" }
}

Ok "Go $(go version)"
Ok "Node $(node -v)"
Ok "pnpm $(pnpm -v)"

# ── 2. 安装前端依赖 ───────────────────────────────────
Info "安装前端依赖..."
Push-Location frontend
& pnpm install
Pop-Location
Ok "前端依赖安装完成"

# ── 3. Go 依赖 ────────────────────────────────────────
Info "下载 Go 依赖..."
& go mod download
Ok "Go 依赖下载完成"

# ── 4. 构建 ───────────────────────────────────────────
Info "开始构建..."
if ($Platform) {
    Info "构建目标平台: $Platform"
    & wails build -platform $Platform
}
else {
    Info "构建当前平台 (Windows)..."
    & wails build
}

if ($LASTEXITCODE -ne 0) {
    Fail "构建失败！退出码: $LASTEXITCODE"
}

Write-Host ""
Ok "═══════════════════════════════════════"
Ok "  ClashGo 构建成功!"
Ok "  产物目录: build\bin\"
Ok "═══════════════════════════════════════"
Get-ChildItem "build\bin" -ErrorAction SilentlyContinue | Format-Table Name, @{N = 'Size(MB)'; E = { [math]::Round($_.Length / 1MB, 2) } } -AutoSize
