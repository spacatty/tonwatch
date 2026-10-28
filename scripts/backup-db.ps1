# Windows twin of backup-db.sh: dumps the Postgres database to backups\tonwatch-<time>.dump
# (pg_dump custom format) and keeps the newest -Keep dumps. Same file format, so restore-db.sh on
# the server takes it as is. Works with the app running or turned down (it starts only the db).
#
#   powershell -ExecutionPolicy Bypass -File scripts\backup-db.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\backup-db.ps1 -Keep 30
param([int]$Keep = 14)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

New-Item -ItemType Directory -Force backups | Out-Null
$name = "tonwatch-$(Get-Date -Format 'yyyyMMdd-HHmmss').dump"
$tmp = "/tmp/$name"

# the stack may be down (a final export before moving to another server): bring up the database
# only, never the backend. -no-recreate leaves a running container alone (e.g. the dev overlay's).
docker compose up -d --wait --no-recreate db | Out-Null
if ($LASTEXITCODE -ne 0) { throw "could not start the database container (is Docker running?)" }

# pg_dump writes the file inside the container and it is copied out: Windows PowerShell would
# mangle binary data piped through >
docker compose exec -T db sh -c ('pg_dump -U $POSTGRES_USER -d $POSTGRES_DB -Fc -f ' + $tmp)
if ($LASTEXITCODE -ne 0) { throw "pg_dump failed" }
docker compose cp "db:$tmp" "backups\$name"
$copied = $LASTEXITCODE
docker compose exec -T db rm -f $tmp
if ($copied -ne 0) { throw "could not copy the dump out of the container" }

$mb = [math]::Round((Get-Item "backups\$name").Length / 1MB, 1)
Write-Output "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') wrote backups\$name ($mb MB)"

Get-ChildItem backups -Filter 'tonwatch-*.dump' | Sort-Object LastWriteTime -Descending | Select-Object -Skip $Keep | Remove-Item
