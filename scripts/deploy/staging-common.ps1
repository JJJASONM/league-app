$ErrorActionPreference = 'Stop'

function Assert-StagingRoot {
    param([string]$StagingRoot)

    $expected = 'C:\inetpub\league-staging'
    $full = [System.IO.Path]::GetFullPath($StagingRoot).TrimEnd('\')
    if (-not $full.Equals($expected, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Unexpected staging root: $full"
    }

    foreach ($name in @('app', 'data', 'logs', 'backups')) {
        New-Item -ItemType Directory -Path (Join-Path $full $name) -Force | Out-Null
    }
    return $full
}

function Get-StagingListener {
    param([int]$Port)

    return Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
}

function Stop-StagingApp {
    param(
        [string]$StagingRoot,
        [int]$Port
    )

    $listener = Get-StagingListener -Port $Port
    if (-not $listener) {
        return
    }

    $process = Get-Process -Id $listener.OwningProcess -ErrorAction Stop
    $allowedPaths = @(
        (Join-Path $StagingRoot 'app\league_app.exe'),
        'C:\Users\admin\source\league-staging\app\league_app.exe'
    )
    $recognized = $allowedPaths | Where-Object {
        $process.Path -and $process.Path.Equals($_, [System.StringComparison]::OrdinalIgnoreCase)
    }
    if (-not $recognized) {
        throw "Port $Port belongs to unrecognized process $($process.Id): $($process.Path)"
    }

    Stop-Process -Id $process.Id -Force
    $process.WaitForExit()
    Start-Sleep -Milliseconds 300
    if (Get-StagingListener -Port $Port) {
        throw "Port $Port is still listening after stopping process $($process.Id)"
    }
}

function Backup-StagingDatabase {
    param([string]$StagingRoot)

    $database = Join-Path $StagingRoot 'data\league.db'
    if (-not (Test-Path -LiteralPath $database)) {
        return $null
    }

    $stamp = Get-Date -Format 'yyyy-MM-dd_HHmmss'
    $backup = Join-Path $StagingRoot "backups\league_$stamp.db"
    Copy-Item -LiteralPath $database -Destination $backup
    return $backup
}

function Start-StagingApp {
    param(
        [string]$StagingRoot,
        [int]$Port
    )

    $executable = Join-Path $StagingRoot 'app\league_app.exe'
    if (-not (Test-Path -LiteralPath $executable)) {
        throw "Staging executable not found: $executable"
    }

    $data = Join-Path $StagingRoot 'data'
    $logs = Join-Path $StagingRoot 'logs'
    $process = Start-Process -FilePath $executable `
        -ArgumentList @('--port', "$Port", '--data', $data) `
        -WorkingDirectory (Split-Path $executable) `
        -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $logs 'app-out.log') `
        -RedirectStandardError (Join-Path $logs 'app-error.log') `
        -PassThru
    return $process
}

function Wait-StagingHealth {
    param(
        [int]$Port,
        [int]$Attempts = 20
    )

    $url = "http://localhost:$Port/api/leagues"
    for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 3
            if ($response.StatusCode -eq 200) {
                return $response
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    throw "Staging API did not become healthy: $url"
}

function Test-PublicStaging {
    param([string]$PublicUrl)

    $curl = Get-Command curl.exe -ErrorAction Stop
    $status = & $curl.Source -k -sS -o NUL -w '%{http_code}' "$PublicUrl/api/leagues"
    if ($LASTEXITCODE -ne 0 -or $status -ne '200') {
        throw "IIS staging health check failed: $PublicUrl/api/leagues returned $status"
    }
    return $status
}

function Restore-StagingDatabase {
    param(
        [string]$StagingRoot,
        [string]$BackupPath
    )

    if (-not $BackupPath -or -not (Test-Path -LiteralPath $BackupPath)) {
        throw 'No database backup is available for restoration.'
    }

    $data = Join-Path $StagingRoot 'data'
    foreach ($name in @('league.db', 'league.db-wal', 'league.db-shm')) {
        $path = Join-Path $data $name
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Force
        }
    }
    Copy-Item -LiteralPath $BackupPath -Destination (Join-Path $data 'league.db')
}
