---
name: deploy-staging
description: Deploy, restart, verify, back up, or seed the League App IIS staging environment at C:\inetpub\league-staging. Use when the user asks to deploy to staging, update the staging site, seed staging data, refresh the staging executable, verify the staging deployment, or roll back a failed League App staging release.
---

# Deploy Staging

Deploy League App to the local IIS staging environment:

```text
IIS: https://league-staging.local
Go app: http://localhost:9000
Root: C:\inetpub\league-staging
Database: C:\inetpub\league-staging\data\league.db
```

## Deploy

1. Read repository guidance and inspect `git status --short`.
2. Tell the user whether the build includes uncommitted changes.
3. Request approval before changing the staging installation.
4. Run:

```powershell
powershell.exe -ExecutionPolicy Bypass -File `
  .codex\skills\deploy-staging\scripts\deploy-staging.ps1 `
  -RepoRoot C:\Users\admin\source\league_app `
  -ConfirmDeploy DEPLOY-STAGING
```

The script tests, checks JavaScript syntax, builds, stops only a recognized
League App staging process, backs up the database, replaces the executable,
restarts it, verifies the private API and IIS URL, and rolls back the
executable when verification fails.

Do not replace `web.config`, `data`, or `logs` during a normal deployment.
Do not commit generated binaries, databases, logs, or backups.

## Seed

Seeding is a separate explicit operation. Explain that it inserts starter
records with `INSERT OR IGNORE`; it does not erase existing records.

1. Request explicit approval to seed staging.
2. Run:

```powershell
powershell.exe -ExecutionPolicy Bypass -File `
  .codex\skills\deploy-staging\scripts\seed-staging.ps1 `
  -ConfirmSeed SEED-STAGING
```

The script stops staging, backs up `league.db`, runs the deployed executable
with `--seed`, restarts it, and verifies that the API returns at least one
league. It restores the database backup if seeding fails.

Never use `--reset-db` as part of this skill. Treat database replacement or
reset as a separate destructive task requiring a fresh explicit request.

## Safety

- Preserve the staging database unless the user explicitly requests seeding.
- Abort if port 9000 belongs to an unrecognized executable.
- Keep development, staging, and production paths distinct.
- Do not stop the development app on port 8080.
- Report backup path, deployed executable path, process ID, and health results.
- If IIS verification fails but the private API succeeds, report IIS as the
  remaining problem rather than claiming the deployment is fully healthy.
