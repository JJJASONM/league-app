param(
    [string]$StagingRoot = 'C:\inetpub\league-staging',
    [int]$Port = 9000,
    [string]$PublicUrl = 'https://league-staging.local',
    [string]$ConfirmSeed
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'staging-common.ps1')

if ($ConfirmSeed -ne 'SEED-STAGING') {
    throw 'Seeding requires -ConfirmSeed SEED-STAGING'
}

$staging = Assert-StagingRoot -StagingRoot $StagingRoot
$executable = Join-Path $staging 'app\league_app.exe'
$data = Join-Path $staging 'data'

Stop-StagingApp -StagingRoot $staging -Port $Port
$backup = Backup-StagingDatabase -StagingRoot $staging

try {
    & $executable --data $data --seed
    if ($LASTEXITCODE -ne 0) {
        throw "Seed command failed with exit code $LASTEXITCODE."
    }

    $process = Start-StagingApp -StagingRoot $staging -Port $Port
    $response = Wait-StagingHealth -Port $Port
    $leagues = $response.Content | ConvertFrom-Json
    if (@($leagues).Count -lt 1) {
        throw 'Seed verification failed: the API returned no leagues.'
    }
    $publicStatus = Test-PublicStaging -PublicUrl $PublicUrl

    [pscustomobject]@{
        Status = 'Seeded'
        ProcessId = $process.Id
        DatabaseBackup = $backup
        LeagueCount = @($leagues).Count
        PublicHealth = "$PublicUrl/api/leagues ($publicStatus)"
    } | Format-List
} catch {
    $failure = $_
    Stop-StagingApp -StagingRoot $staging -Port $Port
    if ($backup) {
        Restore-StagingDatabase -StagingRoot $staging -BackupPath $backup
    }
    $rollbackProcess = Start-StagingApp -StagingRoot $staging -Port $Port
    Wait-StagingHealth -Port $Port | Out-Null
    Write-Warning "Seeding failed; database restored and staging restarted as process $($rollbackProcess.Id)."
    throw $failure
}
