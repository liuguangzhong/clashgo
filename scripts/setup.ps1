# ────────────────────────────────────────────────────────
# ClashGo 环境检查 & 自动安装 (Windows PowerShell)
# Usage: .\scripts\setup.ps1
# ────────────────────────────────────────────────────────
$ErrorActionPreference = "Stop"

function Info { param([string]$msg) Write-Host "[INFO]  $msg" -ForegroundColor Cyan }
function Ok { param([string]$msg) Write-Host "  √ $msg" -ForegroundColor Green }
function Warn { param([string]$msg) Write-Host "  ⚠ $msg" -ForegroundColor Yellow }
function Fail { param([string]$msg) Write-Host "  ✗ $msg" -ForegroundColor Red }

Write-Host ""
Write-Host "═══════════════════════════════════════"
Write-Host "  ClashGo 环境检查"
Write-Host "═══════════════════════════════════════"
Write-Host ""

$Errors = 0

# Go
if (Get-Command go -ErrorAction SilentlyContinue) {
    Ok "Go: $(go version)"
} else {
    Fail "Go 未安装 → https://go.dev/dl/"
    $Errors++
}

# Node.js
if (Get-Command node -ErrorAction SilentlyContinue) {
    Ok "Node.js: $(node -v)"
} else {
    Fail "Node.js 未安装 → https://nodejs.org/"
    $Errors++
}

# pnpm (自动安装)
if (Get-Command pnpm -ErrorAction SilentlyContinue) {
    Ok "pnpm: $(pnpm -v)"
} else {
    Warn "pnpm 未安装，正在通过 npm 安装..."
    try {
        & npm install -g pnpm
        Ok "pnpm: $(pnpm -v)"
    } catch {
        Fail "pnpm 安装失败"
        $Errors++
    }
}

# Wails CLI (自动安装)
if (Get-Command wails -ErrorAction SilentlyContinue) {
    Ok "Wails CLI: installed"
} else {
    Warn "Wails CLI 未安装，正在安装..."
    try {
        & go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
        $env:PATH += ";$(go env GOPATH)\bin"
        if (Get-Command wails -ErrorAction SilentlyContinue) {
            Ok "Wails CLI 安装完成"
        } else {
            Fail "Wails CLI 安装失败"
            $Errors++
        }
    } catch {
        Fail "Wails CLI 安装失败"
        $Errors++
    }
}

# Windows 特有检查: WebView2
Info "检查 Windows 依赖..."
$webview2 = Get-ItemProperty -Path "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" -ErrorAction SilentlyContinue
if ($webview2) {
    Ok "WebView2 Runtime: $($webview2.pv)"
} else {
    $webview2Alt = Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" -ErrorAction SilentlyContinue
    if ($webview2Alt) {
        Ok "WebView2 Runtime: $($webview2Alt.pv)"
    } else {
        Warn "WebView2 Runtime 未检测到 (Windows 10/11 通常已内置)"
        Write-Host "    如果缺少请访问: https://developer.microsoft.com/microsoft-edge/webview2/"
    }
}

# GCC (CGo 需要)
if (Get-Command gcc -ErrorAction SilentlyContinue) {
    Ok "GCC: $(gcc --version | Select-Object -First 1)"
} else {
    Warn "GCC 未安装 (CGo 编译需要)"
    Write-Host "    推荐 TDM-GCC: https://jmeubank.github.io/tdm-gcc/"
    Write-Host "    或 MSYS2: https://www.msys2.org/"
    $Errors++
}

Write-Host ""
if ($Errors -eq 0) {
    Write-Host "═══════════════════════════════════════" -ForegroundColor Green
    Write-Host "  所有依赖已就绪! 🎉" -ForegroundColor Green
    Write-Host "  运行 .\scripts\dev.ps1 开始开发" -ForegroundColor Green
    Write-Host "  运行 .\scripts\build.ps1 进行构建" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════" -ForegroundColor Green
} else {
    Write-Host "═══════════════════════════════════════" -ForegroundColor Red
    Write-Host "  发现 $Errors 个问题，请先修复" -ForegroundColor Red
    Write-Host "═══════════════════════════════════════" -ForegroundColor Red
    exit 1
}
