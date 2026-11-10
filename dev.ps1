# dev.ps1 - Start every service with hot-reloading using Air.
#
# The service list lives in one place: add a directory here and both branches
# below pick it up, so a new service cannot silently miss the local stack.
param(
    [switch]$WithFrontend,
    [switch]$BackendOnly
)

$ErrorActionPreference = "Continue"

$services = @(
    @{ Name = "USER-SVC";       Dir = "backend\user-service";         Cmd = "cd backend/user-service && air" },
    @{ Name = "POST-SVC";       Dir = "backend\post-service";         Cmd = "cd backend/post-service && air" },
    @{ Name = "FOLLOW-SVC";     Dir = "backend\follow-service";       Cmd = "cd backend/follow-service && air" },
    @{ Name = "FEED-SVC";       Dir = "backend\feed-service";         Cmd = "cd backend/feed-service && air" },
    @{ Name = "NOTIFY-SVC";     Dir = "backend\notification-service"; Cmd = "cd backend/notification-service && air" },
    @{ Name = "MODERATION-SVC"; Dir = "backend\moderation-service";   Cmd = "cd backend/moderation-service && air" },
    @{ Name = "CHAT-SVC";       Dir = "backend\chat-service";         Cmd = "cd backend/chat-service && air" },
    @{ Name = "GATEWAY";        Dir = "backend\api-gateway";          Cmd = "cd backend/api-gateway && air" }
)

if ($WithFrontend -or (-not $BackendOnly)) {
    $services += @{ Name = "FRONTEND"; Dir = "frontend"; Cmd = "cd frontend && npm run dev" }
}

Write-Host "Starting microservices $(if (-not $BackendOnly) {'and frontend '} else {''})with hot-reloading..." -ForegroundColor Cyan

# Option A: npx concurrently gives unified, colour-coded logs in one terminal.
if (Get-Command "npx" -ErrorAction SilentlyContinue) {
    $names = ($services | ForEach-Object { $_.Name }) -join ","
    $commands = @($services | ForEach-Object { $_.Cmd })

    Write-Host "Detected npx. Running via concurrently for combined logs..." -ForegroundColor Green
    npx concurrently -n $names -c "cyan,blue,yellow,green,magenta,red,gray,white,black.bgGreen" $commands
    exit
}

# Option B: one Air process per service, so each keeps its own output.
Write-Host "Running services in separate Air processes..." -ForegroundColor Yellow

$processes = @()
$colors = @("Cyan", "Blue", "Yellow", "Green", "Magenta", "Red", "Gray", "White", "DarkCyan")

for ($i = 0; $i -lt $services.Count; $i++) {
    $service = $services[$i]
    if ($service.Name -eq "FRONTEND") {
        $process = Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev" -WorkingDirectory ".\frontend" -PassThru
    } else {
        $process = Start-Process -FilePath "air" -WorkingDirectory ".\$($service.Dir)" -PassThru
    }
    $script:processes += $process

    $color = $colors[$i % $colors.Count]
    Write-Host ("{0,-14} PID: {1}" -f $service.Name, $process.Id) -ForegroundColor $color
}

Write-Host "Press Ctrl+C to stop all services..." -ForegroundColor White

try {
    Wait-Process -Id ($processes | ForEach-Object { $_.Id })
}
finally {
    Write-Host "`nStopping all services..." -ForegroundColor Red
    Stop-Process -Id ($processes | ForEach-Object { $_.Id }) -ErrorAction SilentlyContinue
}
