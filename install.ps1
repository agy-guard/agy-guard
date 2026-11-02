# ==============================================================================
# agy-guard: Quick Plugin Installer & Configuration Setup (Windows PowerShell)
# ==============================================================================
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

Write-Host "    ___    _______  __     _______  _____    ____  ____ " -ForegroundColor Cyan
Write-Host "   /   |  / ____/ \/ /    / ____/ / / /   |  / __ \/ __ \" -ForegroundColor Cyan
Write-Host "  / /| | / / __  \  /    / / __/ / / / /| | / /_/ / / / /" -ForegroundColor Cyan
Write-Host " / ___ |/ /_/ /  / /    / /_/ / /_/ / ___ |/ _, _/ /_/ / " -ForegroundColor Cyan
Write-Host "/_/  |_|\____/  /_/     \____/\____/_/  |_/_/ |_/_____/  " -ForegroundColor Cyan
Write-Host ""
Write-Host "🛡️  agy-guard: Security Classifier for Antigravity CLI (agy)" -ForegroundColor Cyan
Write-Host "------------------------------------------------------------------"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$UserHome = [System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)
$ConfigDir = Join-Path $UserHome ".agy-guard"
$ConfigFile = Join-Path $ConfigDir "config.json"
$PluginDir = Join-Path $UserHome ".gemini\config\plugins\agy-guard"
$LocalBinDir = Join-Path $UserHome ".local\bin"

New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
New-Item -ItemType Directory -Force -Path $LocalBinDir | Out-Null
New-Item -ItemType Directory -Force -Path $PluginDir | Out-Null

# ------------------------------------------------------------------------------
# 1. Build / Locate / Download Binary
# ------------------------------------------------------------------------------
Write-Host "`n[1/4] Preparing agy-guard binary..." -ForegroundColor Yellow

$ExePath = Join-Path $LocalBinDir "agy-guard.exe"

if ((Test-Path "$ScriptDir\cmd\agy-guard\main.go") -and (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Compiling from local source with Go..."
    Push-Location $ScriptDir
    try {
        go build -o "$ScriptDir\agy-guard.exe" .\cmd\agy-guard
        Copy-Item -Path "$ScriptDir\agy-guard.exe" -Destination $ExePath -Force
        Write-Host "✓ Binary built and installed to: $ExePath" -ForegroundColor Green
    }
    finally {
        Pop-Location
    }
} elseif (Test-Path "$ScriptDir\agy-guard.exe") {
    Copy-Item -Path "$ScriptDir\agy-guard.exe" -Destination $ExePath -Force
    Write-Host "✓ Using existing binary installed to: $ExePath" -ForegroundColor Green
} else {
    Write-Host "Downloading pre-compiled binary from GitHub Releases (agy-guard/agy-guard)..."
    try {
        $Arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { "arm64" }
        $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/agy-guard/agy-guard/releases/latest"
        $Asset = $Release.assets | Where-Object { $_.name -like "*windows_$Arch.zip" } | Select-Object -First 1
        if (-not $Asset) {
            $Asset = $Release.assets | Where-Object { $_.name -like "*windows_amd64.zip" } | Select-Object -First 1
        }
        if ($Asset) {
            $ZipPath = Join-Path $env:TEMP "agy-guard.zip"
            $ExtractDir = Join-Path $env:TEMP "agy-guard-extract"
            Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $ZipPath
            Expand-Archive -Path $ZipPath -DestinationPath $ExtractDir -Force
            Copy-Item (Join-Path $ExtractDir "agy-guard.exe") -Destination $ExePath -Force
            Remove-Item $ZipPath -Force -ErrorAction SilentlyContinue
            Remove-Item $ExtractDir -Recurse -Force -ErrorAction SilentlyContinue
            Write-Host "✓ Downloaded and installed to: $ExePath" -ForegroundColor Green
        } else {
            throw "No matching release asset found"
        }
    } catch {
        Write-Error "Failed to download binary: $_. Please install Go or download manually."
        exit 1
    }
}

# ------------------------------------------------------------------------------
# 2. Interactive Model Provider Selection
# ------------------------------------------------------------------------------
Write-Host "`n[2/4] Configure LLM Classifier Provider (Tier 3)..." -ForegroundColor Yellow
Write-Host "Select the model provider to evaluate ambiguous/unrecognized shell commands:"
Write-Host "  1) Google Gemini (Recommended - ultra fast, free API key at https://aistudio.google.com)"
Write-Host "  2) Local Ollama  (100% offline & private, runs locally)"

$ProviderChoice = Read-Host "Enter choice [1/2] (default: 1)"
if ([string]::IsNullOrWhiteSpace($ProviderChoice)) { $ProviderChoice = "1" }

$SelectedProvider = "gemini"
$SelectedModel = "gemini-3.5-flash-lite"
$ApiKey = "YOUR_GEMINI_API_KEY_HERE"
$OllamaUrl = "http://localhost:11434"

if ($ProviderChoice -eq "2") {
    $SelectedProvider = "ollama"
    $InputUrl = Read-Host "Enter Ollama URL [http://localhost:11434]"
    if (-not [string]::IsNullOrWhiteSpace($InputUrl)) { $OllamaUrl = $InputUrl }

    $InputModel = Read-Host "Enter Ollama Model [llama3.2]"
    if (-not [string]::IsNullOrWhiteSpace($InputModel)) { $SelectedModel = $InputModel }
    else { $SelectedModel = "llama3.2" }
    Write-Host "✓ Selected Ollama ($SelectedModel) at $OllamaUrl" -ForegroundColor Green
} else {
    $SelectedProvider = "gemini"
    $InputModel = Read-Host "Enter Gemini Model [gemini-3.5-flash]"
    if (-not [string]::IsNullOrWhiteSpace($InputModel)) { $SelectedModel = $InputModel } else { $SelectedModel = "gemini-3.5-flash" }

    # Check env
    $EnvKey = $env:GEMINI_API_KEY
    if (-not [string]::IsNullOrWhiteSpace($EnvKey)) {
        $KeyMask = $EnvKey.Substring(0, [Math]::Min(6, $EnvKey.Length)) + "..."
        $UseEnv = Read-Host "Found GEMINI_API_KEY in environment ($KeyMask). Use this? [Y/n]"
        if ([string]::IsNullOrWhiteSpace($UseEnv) -or $UseEnv -match "^[Yy]") {
            $ApiKey = $EnvKey
        }
    }

    if ($ApiKey -eq "YOUR_GEMINI_API_KEY_HERE") {
        Write-Host "Get a free Gemini API key from: https://aistudio.google.com" -ForegroundColor Cyan
        $InputKey = Read-Host "Enter your Gemini API Key"
        if (-not [string]::IsNullOrWhiteSpace($InputKey)) {
            $ApiKey = $InputKey.Trim()
        } else {
            Write-Host "Warning: No API key provided. You can add it later to ~/.agy-guard/config.json" -ForegroundColor Yellow
        }
    }
    Write-Host "✓ Selected Gemini ($SelectedModel)" -ForegroundColor Green
}

# ------------------------------------------------------------------------------
# 3. Execution Mode Selection (Interactive vs Autonomous AFK)
# ------------------------------------------------------------------------------
Write-Host "`n[3/5] Select Execution Mode..." -ForegroundColor Yellow
Write-Host "How should agy-guard handle ambiguous or state-changing commands?"
Write-Host "  1) Interactive Mode  (Recommended - pauses and asks confirmation on suspicious actions)"
Write-Host "  2) Autonomous / AFK  (Zero prompts - agent autonomously adapts or skips when blocked)"

$ModeChoice = Read-Host "Enter choice [1/2] (default: 1)"
if (-not $ModeChoice) { $ModeChoice = "1" }

if ($ModeChoice -eq "2") {
    $UnattendedMode = $true
    $ModeLabel = "Autonomous / AFK Mode (Zero Prompts)"
    Write-Host "✓ Selected Autonomous / AFK Mode" -ForegroundColor Yellow
} else {
    $UnattendedMode = $false
    $ModeLabel = "Interactive Mode (Confirmations Enabled)"
    Write-Host "✓ Selected Interactive Mode" -ForegroundColor Green
}

# ------------------------------------------------------------------------------
# 4. Write / Update ~/.agy-guard/config.json
# ------------------------------------------------------------------------------
Write-Host "`n[4/5] Writing configuration to $ConfigFile..." -ForegroundColor Yellow

$EscapedExePath = $ExePath.Replace("\", "\\")

$ConfigObj = [ordered]@{
    "provider"                   = $SelectedProvider
    "model"                      = $SelectedModel
    "api_key"                    = $ApiKey
    "ollama_url"                 = $OllamaUrl
    "timeout_sec"                = 15
    "include_transcript_context" = $true
    "enable_audit_log"           = $true
    "enable_cache"               = $true
    "cache_ttl_minutes"          = 10
    "unattended_mode"            = $UnattendedMode
    "max_retries_before_ask"     = 2
    "static_rules"               = [ordered]@{
        "allow_prefixes"  = @(
            "go test", "go build", "go vet", "go list", "go mod tidy", "go fmt", "go env", "go version", "go doc", "golangci-lint",
            "git status", "git diff", "git log", "git branch", "git show", "git rev-parse", "git remote", "git tag", "git ls-files", "git describe", "git blame",
            "cargo check", "cargo test", "cargo build", "cargo clippy", "cargo fmt", "cargo doc",
            "npm test", "npm run test", "npm run lint", "npm run build", "npm run typecheck", "npm ls",
            "yarn test", "pnpm test", "npx eslint", "npx tsc", "npx prettier --check", "bun test",
            "pytest", "python -m unittest", "python3 -m unittest", "python -m pytest", "python3 -m pytest", "ruff check", "black --check", "mypy", "flake8",
            "ls", "cat", "head", "tail", "grep", "egrep", "fgrep", "rg", "ag", "find", "pwd", "echo", "printf", "wc", "uname", "which", "stat", "file", "diff", "jq", "sort", "uniq", "tree"
        )
        "deny_substrings" = @(
            ".env", "id_rsa", "id_ed25519", "id_ecdsa", "gh auth token", "aws_secret_access_key", "google_api_key", "gemini_api_key", "openai_api_key", "anthropic_api_key", "authorization: bearer", "bearer eyj", "private key-----", ".git-credentials", "/etc/shadow", "/etc/sudoers",
            "sudo ", "doas ", "su -", "su ", "runas ", "gsudo ",
            "| sh", "| bash", "| zsh", "| dash", "| powershell", "| pwsh", "| cmd", "|sh", "|bash", "|zsh", "|pwsh",
            "git push -f", "git push --force", "git reset --hard", "git reset --merge", "git clean -f", "git clean -fd", "git clean -fx", "git checkout .", "git restore .", "git branch -d", "git stash drop", "git stash clear",
            ".git/", ".bashrc", ".zshrc", ".profile", ".bash_profile", "config.fish",
            "rm -rf /", "rm -rf ~", "rm -rf `$home", "rm -rf *", "rm -fr /", "rm -fr ~", "rm -fr `$home", "rm -fr *",
            "mkfs", "dd if=", "format c:", "Format-Volume", "Remove-Item -Recurse -Force C:\", "Stop-Computer", "shutdown ", "reboot ", ":(){ :|:& };:"
        )
    }
}

$JsonConfig = $ConfigObj | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText($ConfigFile, $JsonConfig, [System.Text.Encoding]::UTF8)
Write-Host "✓ Config saved at: $ConfigFile" -ForegroundColor Green

# ------------------------------------------------------------------------------
# 5. Register Plugin in Antigravity CLI
# ------------------------------------------------------------------------------
Write-Host "`n[5/5] Registering agy-guard as native Antigravity plugin..." -ForegroundColor Yellow
& $ExePath init --plugin

# ------------------------------------------------------------------------------
# Verification
# ------------------------------------------------------------------------------
Write-Host "`nRunning verification self-test..." -ForegroundColor Yellow
& $ExePath dry-run "go test ./..."

Write-Host "`n==================================================================" -ForegroundColor Green
Write-Host "🎉 agy-guard installed and configured successfully!" -ForegroundColor Green
Write-Host "==================================================================" -ForegroundColor Green
Write-Host "• Binary path:      $ExePath" -ForegroundColor Cyan
Write-Host "• Plugin location:  $PluginDir" -ForegroundColor Cyan
Write-Host "• Execution mode:   $ModeLabel" -ForegroundColor Cyan
Write-Host "• Configuration:    $ConfigFile" -ForegroundColor Cyan
Write-Host "• Audit logs:       $ConfigDir\audit" -ForegroundColor Cyan
Write-Host ""
Write-Host "Antigravity CLI ('agy') will now automatically route PreToolUse events through agy-guard."
Write-Host "You can test any command directly at any time:"
Write-Host "  agy-guard dry-run `"<command>`"" -ForegroundColor White
Write-Host ""
Write-Host "To uninstall at any time:"
Write-Host "  .\uninstall.ps1  (or: agy-guard uninstall)" -ForegroundColor Cyan
Write-Host ""
