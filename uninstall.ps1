# ==============================================================================
# agy-guard: Plugin Uninstaller & Cleanup Script (Windows PowerShell)
# ==============================================================================
[CmdletBinding()]
param(
    [switch]$Purge,
    [switch]$Force
)

$ErrorActionPreference = "Continue"

Write-Host "    ___    _______  __     _______  _____    ____  ____ " -ForegroundColor Cyan
Write-Host "   /   |  / ____/ \/ /    / ____/ / / /   |  / __ \/ __ \" -ForegroundColor Cyan
Write-Host "  / /| | / / __  \  /    / / __/ / / / /| | / /_/ / / / /" -ForegroundColor Cyan
Write-Host " / ___ |/ /_/ /  / /    / /_/ / /_/ / ___ |/ _, _/ /_/ / " -ForegroundColor Cyan
Write-Host "/_/  |_|\____/  /_/     \____/\____/_/  |_/_/ |_/_____/  " -ForegroundColor Cyan
Write-Host ""
Write-Host "🛡️  agy-guard Uninstaller" -ForegroundColor Cyan
Write-Host "------------------------------------------------------------------"

$UserHome = [System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)
$ConfigDir = Join-Path $UserHome ".agy-guard"
$PluginDir = Join-Path $UserHome ".gemini\config\plugins\agy-guard"
$HooksFile = Join-Path $UserHome ".gemini\config\hooks.json"
$LocalBin = Join-Path $UserHome ".local\bin\agy-guard.exe"
$GoBin = Join-Path $UserHome "go\bin\agy-guard.exe"

# ------------------------------------------------------------------------------
# 1. Unregister hooks from Antigravity CLI
# ------------------------------------------------------------------------------
Write-Host "`n[1/4] Unregistering hooks from Antigravity CLI..." -ForegroundColor Yellow

if (Test-Path $HooksFile) {
    try {
        $content = Get-Content -Path $HooksFile -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($content.PSObject.Properties['agy-guard']) {
            $content.PSObject.Properties.Remove('agy-guard')
            $updatedJson = $content | ConvertTo-Json -Depth 5
            [System.IO.File]::WriteAllText($HooksFile, $updatedJson, [System.Text.Encoding]::UTF8)
            Write-Host "✓ Cleaned agy-guard from: $HooksFile" -ForegroundColor Green
        } else {
            Write-Host "• agy-guard was not registered in hooks.json" -ForegroundColor Gray
        }
    } catch {
        Write-Warning "Could not parse or update $HooksFile: $_"
    }
} else {
    Write-Host "• hooks.json not found, skipping." -ForegroundColor Gray
}

# ------------------------------------------------------------------------------
# 2. Remove native plugin directory
# ------------------------------------------------------------------------------
Write-Host "`n[2/4] Removing native plugin directory..." -ForegroundColor Yellow
if (Test-Path $PluginDir) {
    Remove-Item -Path $PluginDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "✓ Removed: $PluginDir" -ForegroundColor Green
} else {
    Write-Host "• Plugin directory not found, skipping." -ForegroundColor Gray
}

# ------------------------------------------------------------------------------
# 3. Remove executable binary
# ------------------------------------------------------------------------------
Write-Host "`n[3/4] Removing executable binary..." -ForegroundColor Yellow
$removedAny = $false

if (Test-Path $LocalBin) {
    Remove-Item -Path $LocalBin -Force -ErrorAction SilentlyContinue
    Write-Host "✓ Removed: $LocalBin" -ForegroundColor Green
    $removedAny = $true
}

if (Test-Path $GoBin) {
    Remove-Item -Path $GoBin -Force -ErrorAction SilentlyContinue
    Write-Host "✓ Removed: $GoBin" -ForegroundColor Green
    $removedAny = $true
}

if (-not $removedAny) {
    Write-Host "• No binary found in standard locations." -ForegroundColor Gray
}

# ------------------------------------------------------------------------------
# 4. Remove config / audit logs if requested
# ------------------------------------------------------------------------------
Write-Host "`n[4/4] Configuration & Audit Logs cleanup..." -ForegroundColor Yellow
if (Test-Path $ConfigDir) {
    $doPurge = $Purge.IsPresent

    if (-not $doPurge -and -not $Force.IsPresent) {
        $ans = Read-Host "Do you want to delete ~/.agy-guard (config.json, audit logs, and cache)? [y/N]"
        if ($ans -match "^[Yy]") {
            $doPurge = $true
        }
    }

    if ($doPurge) {
        Remove-Item -Path $ConfigDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "✓ Removed configuration and audit directory: $ConfigDir" -ForegroundColor Green
    } else {
        Write-Host "• Kept configuration and logs in: $ConfigDir" -ForegroundColor Yellow
    }
} else {
    Write-Host "• $ConfigDir does not exist." -ForegroundColor Gray
}

Write-Host "`n==================================================================" -ForegroundColor Green
Write-Host "✓ agy-guard uninstalled successfully!" -ForegroundColor Green
Write-Host "==================================================================" -ForegroundColor Green
Write-Host "Antigravity CLI ('agy') has been restored to default behavior.`n"
