# ────────────────────────────────────────────────────────
# ClashGo 环境检查 & 自动安装 (Windows PowerShell)
# Usage: .\scripts\setup.ps1
# ────────────────────────────────────────────────────────
# 不使用 $ErrorActionPreference = "Stop"，让脚本能收集所有错误

function Info { param([string]$msg) Write-Host "[INFO]  $msg" -ForegroundColor Cyan }
function Ok { param([string]$msg) Write-Host "  √ $msg" -ForegroundColor Green }
function Warn { param([string]$msg) Write-Host "  ⚠ $msg" -ForegroundColor Yellow }
function Fail { param([string]$msg) Write-Host "  ✗ $msg" -ForegroundColor Red }

Write-Host ""
Write-Host "═══════════════════════════════════════"
Write-Host "  ClashGo 环境检查 & 依赖安装"
Write-Host "═══════════════════════════════════════"
Write-Host ""

$Errors = 0
$HasGo = $false
$HasNode = $false

# ── 1. Go ──────────────────────────────────────────────
if (Get-Command go -ErrorAction SilentlyContinue) {
    Ok "Go: $(go version)"
    $HasGo = $true
}
else {
    Fail "Go 未安装 → https://go.dev/dl/"
    $Errors++
}

# ── 2. Node.js ─────────────────────────────────────────
if (Get-Command node -ErrorAction SilentlyContinue) {
    Ok "Node.js: $(node -v)"
    $HasNode = $true
}
else {
    Fail "Node.js 未安装 → https://nodejs.org/"
    $Errors++
}

# ── 3. pnpm (自动安装，依赖 npm/Node.js) ──────────────
if (Get-Command pnpm -ErrorAction SilentlyContinue) {
    Ok "pnpm: $(pnpm -v)"
}
else {
    if ($HasNode -and (Get-Command npm -ErrorAction SilentlyContinue)) {
        Warn "pnpm 未安装，正在通过 npm 安装..."
        & npm install -g pnpm 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0 -and (Get-Command pnpm -ErrorAction SilentlyContinue)) {
            Ok "pnpm: $(pnpm -v)"
        }
        else {
            Fail "pnpm 自动安装失败，请手动运行: npm install -g pnpm"
            $Errors++
        }
    }
    else {
        Fail "pnpm 未安装 (需要先安装 Node.js 后才能自动安装)"
        $Errors++
    }
}

# ── 4. Wails CLI (自动安装，依赖 Go) ──────────────────
if (Get-Command wails -ErrorAction SilentlyContinue) {
    Ok "Wails CLI: installed"
}
else {
    if ($HasGo) {
        Warn "Wails CLI 未安装，正在通过 go install 安装..."
        & go install github.com/wailsapp/wails/v2/cmd/wails@latest 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) {
            $env:PATH += ";$(go env GOPATH)\bin"
            if (Get-Command wails -ErrorAction SilentlyContinue) {
                Ok "Wails CLI 安装完成"
            }
            else {
                Fail "Wails CLI 安装完成但未在 PATH 中找到，请将 $(go env GOPATH)\bin 加入系统 PATH"
                $Errors++
            }
        }
        else {
            Fail "Wails CLI 安装失败"
            $Errors++
        }
    }
    else {
        Fail "Wails CLI 未安装 (需要先安装 Go 后才能自动安装)"
        $Errors++
    }
}

# ── 5. Windows 特有检查 ───────────────────────────────
Info "检查 Windows 依赖..."

# WebView2
$webview2 = Get-ItemProperty -Path "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" -ErrorAction SilentlyContinue
if (-not $webview2) {
    $webview2 = Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" -ErrorAction SilentlyContinue
}
if ($webview2) {
    Ok "WebView2 Runtime: $($webview2.pv)"
}
else {
    Warn "WebView2 Runtime 未检测到 (Windows 10/11 通常已内置)"
    Write-Host "    如果缺少请访问: https://developer.microsoft.com/microsoft-edge/webview2/"
}

# GCC (CGo 需要)
if (Get-Command gcc -ErrorAction SilentlyContinue) {
    Ok "GCC: $(gcc --version | Select-Object -First 1)"
}
else {
    Fail "GCC 未安装 (CGo 编译需要)"
    Write-Host "    推荐 TDM-GCC: https://jmeubank.github.io/tdm-gcc/"
    Write-Host "    或 MSYS2: https://www.msys2.org/"
    $Errors++
}

# ── 结果汇总 ──────────────────────────────────────────
Write-Host ""
if ($Errors -eq 0) {
    Write-Host "═══════════════════════════════════════" -ForegroundColor Green
    Write-Host "  所有依赖已就绪! 🎉" -ForegroundColor Green
    Write-Host "  运行 .\scripts\dev.ps1 开始开发" -ForegroundColor Green
    Write-Host "  运行 .\scripts\build.ps1 进行构建" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════" -ForegroundColor Green
}
else {
    Write-Host "═══════════════════════════════════════" -ForegroundColor Red
    Write-Host "  发现 $Errors 个问题，请先修复后重新运行" -ForegroundColor Red
    Write-Host "  .\scripts\setup.ps1" -ForegroundColor Red
    Write-Host "═══════════════════════════════════════" -ForegroundColor Red
    exit 1
}
