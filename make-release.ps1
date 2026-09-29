# EdgeAgent Hub 发布打包脚本
# 用法: powershell -ExecutionPolicy Bypass -File make-release.ps1
# 用法: powershell -ExecutionPolicy Bypass -File make-release.ps1 -Version 1.0.0
# 产出: release/edgeagent-hub-<version>-<platform>.zip
#
# 流程:
#   1. 构建前端 (Vue3 → web/dist/)
#   2. 交叉编译多平台二进制 (CGO_ENABLED=0 纯 Go，无 C 依赖)
#   3. 打包发布 zip（二进制 + install.sh + config.yaml + README.md）
#   4. 生成 SHA256 校验文件

param(
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ProjectRoot

Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  EdgeAgent Hub v$Version Release Builder" -ForegroundColor Cyan
Write-Host "  边缘智能体管理平台" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""

# ============================================================
# 0. 前置检查
# ============================================================
Write-Host "[0/5] Checking prerequisites..." -ForegroundColor Green

$goOk = $false
try { $goVer = (go version 2>$null); if ($goVer) { $goOk = $true } } catch {}
if (-not $goOk) {
    Write-Host "ERROR: Go is not installed or not in PATH" -ForegroundColor Red
    Write-Host "  Please install Go 1.24+ from https://go.dev/dl/" -ForegroundColor Yellow
    exit 1
}
Write-Host "  Go: $goVer" -ForegroundColor DarkGray

$nodeOk = $false
try { $nodeVer = (node --version 2>$null); if ($nodeVer) { $nodeOk = $true } } catch {}
if (-not $nodeOk) {
    Write-Host "ERROR: Node.js is not installed or not in PATH" -ForegroundColor Red
    Write-Host "  Please install Node.js 18+ from https://nodejs.org/" -ForegroundColor Yellow
    exit 1
}
Write-Host "  Node: $nodeVer" -ForegroundColor DarkGray

$npmOk = $false
try { $npmVer = (npm --version 2>$null); if ($npmVer) { $npmOk = $true } } catch {}
if (-not $npmOk) {
    Write-Host "ERROR: npm is not installed or not in PATH" -ForegroundColor Red
    exit 1
}
Write-Host "  npm: v$npmVer" -ForegroundColor DarkGray

# ============================================================
# 1. 构建前端 (web-vue → web/dist)
# ============================================================
Write-Host "[1/5] Building frontend..." -ForegroundColor Green
Push-Location web-vue

if (-not (Test-Path "node_modules")) {
    Write-Host "  Installing npm dependencies..." -ForegroundColor DarkGray
    $npmInstall = Start-Process -FilePath "cmd.exe" -ArgumentList "/c npm install --legacy-peer-deps" -NoNewWindow -Wait -PassThru
    if ($npmInstall.ExitCode -ne 0) {
        Write-Host "  Retrying npm install without legacy-peer-deps..." -ForegroundColor DarkGray
        $npmInstall = Start-Process -FilePath "cmd.exe" -ArgumentList "/c npm install" -NoNewWindow -Wait -PassThru
        if ($npmInstall.ExitCode -ne 0) {
            Write-Host "ERROR: npm install failed (exit $($npmInstall.ExitCode))" -ForegroundColor Red
            Pop-Location
            exit 1
        }
    }
}

Write-Host "  Building Vue3 frontend..." -ForegroundColor DarkGray
$viteBuild = Start-Process -FilePath "cmd.exe" -ArgumentList "/c npx vite build" -NoNewWindow -Wait -PassThru
if ($viteBuild.ExitCode -ne 0 -or -not (Test-Path "../web/dist/index.html")) {
    Write-Host "  First vite build failed (exit $($viteBuild.ExitCode)), retrying..." -ForegroundColor DarkGray
    Start-Sleep -Seconds 1
    $viteBuild = Start-Process -FilePath "cmd.exe" -ArgumentList "/c npx vite build" -NoNewWindow -Wait -PassThru
}

if (-not (Test-Path "../web/dist/index.html")) {
    Write-Host "ERROR: Frontend build failed - web/dist/index.html not found" -ForegroundColor Red
    Write-Host "  Try running manually: cd web-vue && npm run build" -ForegroundColor Yellow
    Pop-Location
    exit 1
}
Pop-Location
Write-Host "  Frontend built OK -> web/dist/" -ForegroundColor DarkGray

# ============================================================
# 2. 嵌入前端静态文件到 Go 二进制 (embed)
# ============================================================
# web/dist/ 已经通过 Go embed 嵌入到二进制中，无需单独打包

# ============================================================
# 3. 编译多平台二进制 (CGO_ENABLED=0 纯 Go)
# ============================================================
Write-Host "[2/5] Building multi-platform binaries..." -ForegroundColor Green

$distDir = "dist"
New-Item -ItemType Directory -Path $distDir -Force | Out-Null

$buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$gitCommit = "unknown"
try {
    $gitCommit = (git rev-parse --short HEAD 2>$null).Trim()
} catch {}

$ldflags = "-s -w -X main.version=$Version -X main.buildTime=$buildTime"
if ($gitCommit -ne "unknown") {
    $ldflags += " -X main.gitCommit=$gitCommit"
}

$binaries = @{}

# --- Linux amd64 ---
$env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"; $env:GOPROXY = "https://goproxy.cn,direct"
$out = "$distDir/edgeagent-hub-linux-amd64"
Write-Host "  Building linux/amd64..." -ForegroundColor DarkGray
$prevEAP = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
& go build -ldflags="$ldflags" -o $out ./cmd/edgehub 2>&1 | Out-Null
$buildExit = $LASTEXITCODE; $ErrorActionPreference = $prevEAP
if ($buildExit -ne 0 -or -not (Test-Path $out)) { Write-Host "ERROR: Build failed for linux-amd64 (exit $buildExit)" -ForegroundColor Red; exit 1 }
$binaries["linux-amd64"] = $out
Write-Host "    -> edgeagent-hub-linux-amd64 ($([math]::Round((Get-Item $out).Length / 1MB, 1)) MB)" -ForegroundColor Yellow

# --- Linux arm64 ---
$env:GOOS = "linux"; $env:GOARCH = "arm64"; $env:CGO_ENABLED = "0"; $env:GOPROXY = "https://goproxy.cn,direct"
$out = "$distDir/edgeagent-hub-linux-arm64"
Write-Host "  Building linux/arm64..." -ForegroundColor DarkGray
$prevEAP = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
& go build -ldflags="$ldflags" -o $out ./cmd/edgehub 2>&1 | Out-Null
$buildExit = $LASTEXITCODE; $ErrorActionPreference = $prevEAP
if ($buildExit -ne 0 -or -not (Test-Path $out)) { Write-Host "ERROR: Build failed for linux-arm64 (exit $buildExit)" -ForegroundColor Red; exit 1 }
$binaries["linux-arm64"] = $out
Write-Host "    -> edgeagent-hub-linux-arm64 ($([math]::Round((Get-Item $out).Length / 1MB, 1)) MB)" -ForegroundColor Yellow

# --- Windows amd64 ---
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"; $env:GOPROXY = "https://goproxy.cn,direct"
$out = "$distDir/edgeagent-hub-windows-amd64.exe"
Write-Host "  Building windows/amd64..." -ForegroundColor DarkGray
$prevEAP = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
& go build -ldflags="$ldflags" -o $out ./cmd/edgehub 2>&1 | Out-Null
$buildExit = $LASTEXITCODE; $ErrorActionPreference = $prevEAP
if ($buildExit -ne 0 -or -not (Test-Path $out)) { Write-Host "ERROR: Build failed for windows-amd64 (exit $buildExit)" -ForegroundColor Red; exit 1 }
$binaries["windows-amd64"] = $out
Write-Host "    -> edgeagent-hub-windows-amd64.exe ($([math]::Round((Get-Item $out).Length / 1MB, 1)) MB)" -ForegroundColor Yellow

# 清理交叉编译环境变量
$env:GOOS = ""; $env:GOARCH = ""; $env:CGO_ENABLED = ""
Write-Host "  All binaries built OK" -ForegroundColor DarkGray

# ============================================================
# 4. 打包发布 zip
# ============================================================
Write-Host "[3/5] Preparing release packages..." -ForegroundColor Green

$releaseDir = "release"
New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null

# 公共文件 (所有平台都包含)
$commonFiles = @(
    @{ Src="configs/config.yaml";      Dst="config.yaml" },
    @{ Src="deploy/install.sh";         Dst="install.sh" },
    @{ Src="deploy/uninstall.sh";       Dst="uninstall.sh" },
    @{ Src="README.md";                 Dst="README.md" }
)

# Linux 额外文件
$linuxExtraFiles = @(
    @{ Src="deploy/edgeagent-hub.service"; Dst="edgeagent-hub.service" },
    @{ Src="deploy/edgeagent-hub-env";     Dst="edgeagent-hub-env" }
)

function Package-Zip($tag, $binKey, $isWinHost, $extraFiles) {
    $pkgDir = "release/edgeagent-hub-$tag-pkg"
    New-Item -ItemType Directory -Path $pkgDir -Force | Out-Null

    # 复制二进制
    $ext = if ($isWinHost) { ".exe" } else { "" }
    Copy-Item $binaries[$binKey] "$pkgDir/edgeagent-hub$ext" -Force

    # 复制公共文件
    foreach ($f in $commonFiles) {
        if (Test-Path $f.Src) { Copy-Item $f.Src "$pkgDir/$($f.Dst)" -Force }
    }

    # 复制额外文件
    if ($extraFiles) {
        foreach ($f in $extraFiles) {
            if (Test-Path $f.Src) { Copy-Item $f.Src "$pkgDir/$($f.Dst)" -Force }
        }
    }

    # 打 zip 包
    $zipball = "release/edgeagent-hub-$Version-$tag.zip"
    Write-Host "  Creating $zipball..." -ForegroundColor DarkGray
    Compress-Archive -Path "$pkgDir/*" -DestinationPath $zipball -Force

    $size = [math]::Round((Get-Item $zipball).Length / 1MB, 1)
    Write-Host "    -> $zipball ($size MB)" -ForegroundColor Yellow
}

# 打包 Linux amd64
Package-Zip -tag "linux-amd64" -binKey "linux-amd64" -isWinHost $false -extraFiles $linuxExtraFiles

# 打包 Linux arm64
Package-Zip -tag "linux-arm64" -binKey "linux-arm64" -isWinHost $false -extraFiles $linuxExtraFiles

# 打包 Windows amd64（不包含 Linux 脚本）
Package-Zip -tag "windows-amd64" -binKey "windows-amd64" -isWinHost $true -extraFiles $null

# ============================================================
# 5. 生成 SHA256 校验文件
# ============================================================
Write-Host "[4/5] Generating checksums..." -ForegroundColor Green
Push-Location $releaseDir
$hashes = Get-ChildItem -Filter "*.zip" | ForEach-Object {
    $hash = (Get-FileHash $_.Name -Algorithm SHA256).Hash
    "$hash  $($_.Name)"
}
$hashes | Out-File -Encoding ASCII -FilePath "checksums.txt"
Pop-Location

# ============================================================
# 输出结果
# ============================================================
Write-Host "[5/5] Done!" -ForegroundColor Green
Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "  Release packages created:" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Green
Get-ChildItem $releaseDir -Filter "*.zip" | ForEach-Object {
    $size = if ($_.Length -gt 1MB) { "$([math]::Round($_.Length/1MB,1)) MB" } else { "$([math]::Round($_.Length/1KB,0)) KB" }
    Write-Host "  release/$($_.Name)  ($size)" -ForegroundColor White
}
Write-Host ""
Write-Host "Each package contains:" -ForegroundColor Cyan
Write-Host "  edgeagent-hub            <- binary" -ForegroundColor DarkGray
Write-Host "  config.yaml              <- default config" -ForegroundColor DarkGray
Write-Host "  install.sh               <- one-click installer (Linux)" -ForegroundColor DarkGray
Write-Host "  uninstall.sh             <- uninstaller (Linux)" -ForegroundColor DarkGray
Write-Host "  edgeagent-hub.service    <- systemd service (Linux)" -ForegroundColor DarkGray
Write-Host "  edgeagent-hub-env        <- env file (Linux)" -ForegroundColor DarkGray
Write-Host "  README.md                <- full documentation" -ForegroundColor DarkGray
Write-Host ""
Write-Host "Linux deploy:" -ForegroundColor Yellow
Write-Host "  1. Upload zip to server" -ForegroundColor Yellow
Write-Host "  2. unzip edgeagent-hub-$Version-linux-amd64.zip" -ForegroundColor Yellow
Write-Host "  3. sudo bash install.sh" -ForegroundColor Yellow
Write-Host ""
Write-Host "Windows deploy:" -ForegroundColor Yellow
Write-Host "  1. Unzip edgeagent-hub-$Version-windows-amd64.zip" -ForegroundColor Yellow
Write-Host "  2. Open PowerShell as Administrator" -ForegroundColor Yellow
Write-Host "  3. .\edgeagent-hub.exe -config config.yaml" -ForegroundColor Yellow
Write-Host ""
