param(
    [string]$RepoRoot = 'C:\Users\admin\source\league_app',
    [string]$StagingRoot = 'C:\inetpub\league-staging',
    [int]$Port = 9000,
    [string]$PublicUrl = 'http://league-staging.local',
    [string]$ConfirmDeploy
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'staging-common.ps1')

if ($ConfirmDeploy -ne 'DEPLOY-STAGING') {
    throw 'Deployment requires -ConfirmDeploy DEPLOY-STAGING'
}

$repo = (Resolve-Path -LiteralPath $RepoRoot).Path
$staging = Assert-StagingRoot -StagingRoot $StagingRoot
$artifact = Join-Path $repo 'dist\league_app.staging.exe'
$target = Join-Path $staging 'app\league_app.exe'
$previous = Join-Path $staging 'app\league_app.previous.exe'
$next = Join-Path $staging 'app\league_app.next.exe'

Push-Location $repo
try {
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }

    & node --check web\app.js
    if ($LASTEXITCODE -ne 0) { throw 'web/app.js syntax check failed.' }

    & node --check web\domains\rules\rules-domain.js
    if ($LASTEXITCODE -ne 0) { throw 'Rules module syntax check failed.' }

    New-Item -ItemType Directory -Path (Split-Path $artifact) -Force | Out-Null
    & go build -o $artifact .
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
} finally {
    Pop-Location
}

Stop-StagingApp -StagingRoot $staging -Port $Port
$backup = Backup-StagingDatabase -StagingRoot $staging

try {
    Copy-Item -LiteralPath $artifact -Destination $next -Force
    if (Test-Path -LiteralPath $previous) {
        Remove-Item -LiteralPath $previous -Force
    }
    if (Test-Path -LiteralPath $target) {
        Move-Item -LiteralPath $target -Destination $previous
    }
    Move-Item -LiteralPath $next -Destination $target

    $process = Start-StagingApp -StagingRoot $staging -Port $Port
    Wait-StagingHealth -Port $Port | Out-Null
    $publicStatus = Test-PublicStaging -PublicUrl $PublicUrl

    [pscustomobject]@{
        Status = 'Deployed'
        Executable = $target
        ProcessId = $process.Id
        DatabaseBackup = $backup
        PrivateHealth = "http://localhost:$Port/api/leagues"
        PublicHealth = "$PublicUrl/api/leagues ($publicStatus)"
    } | Format-List
} catch {
    $failure = $_
    Stop-StagingApp -StagingRoot $staging -Port $Port

    if (Test-Path -LiteralPath $previous) {
        if (Test-Path -LiteralPath $target) {
            Remove-Item -LiteralPath $target -Force
        }
        Move-Item -LiteralPath $previous -Destination $target
        $rollbackProcess = Start-StagingApp -StagingRoot $staging -Port $Port
        Wait-StagingHealth -Port $Port | Out-Null
        Write-Warning "Deployment failed; executable rolled back to process $($rollbackProcess.Id)."
    }
    throw $failure
} finally {
    if (Test-Path -LiteralPath $next) {
        Remove-Item -LiteralPath $next -Force
    }
}
