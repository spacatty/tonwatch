# Starts the Go backend and the Next.js dashboard in two windows.
$root = $PSScriptRoot
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$root\backend'; go run ./cmd/server"
if (-not (Test-Path "$root\frontend\node_modules")) { npm --prefix "$root\frontend" install }
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$root\frontend'; npm run dev"
Start-Sleep 5
Start-Process "http://localhost:3000"
