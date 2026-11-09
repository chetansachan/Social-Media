# dev.ps1 - Start every backend service with hot-reloading using Air.
#
# The service list lives in one place: add a directory here and both branches
# below pick it up, so a new service cannot silently miss the local stack.
$ErrorActionPreference = "Continue"

$services = @(
    @{ Name = "USER-SVC";       Dir = "backend\user-service" },
    @{ Name = "POST-SVC";       Dir = "backend\post-service" },
    @{ Name = "FOLLOW-SVC";     Dir = "backend\follow-service" },
    @{ Name = "FEED-SVC";       Dir = "backend\feed-service" },
    @{ Name = "NOTIFY-SVC";     Dir = "backend\notification-service" },
    @{ Name = "MODERATION-SVC"; Dir = "backend\moderation-service" },
    @{ Name = "CHAT-SVC";       Dir = "backend\chat-service" },
    @{ Name = "GATEWAY";        Dir = "backend\api-gateway" }
)

Write-Host "Starting microservices with Air..." -ForegroundColor Cyan

# Option A: npx concurrently gives unified, colour-coded logs in one terminal.
if (Get-Command "npx" -ErrorAction SilentlyContinue) {
    $names = ($services | ForEach-Object { $_.Name }) -join ","
    $commands = ($services | ForEach-Object { "cd $($_.Dir) && air" }) -join "`n`t"

    Write-Host "Detected npx. Running via concurrently for combined logs..." -ForegroundColor Green
    npx concurrently -n $names -c "cyan,blue,yellow,green,magenta,red,gray,white" $commands
    exit
}

# Option B: one Air process per service, so each keeps its own output.
Write-Host "Running services in separate Air processes..." -ForegroundColor Yellow

$processes = @()
$colors = @("Cyan", "Blue", "Yellow", "Green", "Magenta", "Red", "Gray", "White")

for ($i = 0; $i -lt $services.Count; $i++) {
    $service = $services[$i]
    $process = Start-Process -FilePath "air" -WorkingDirectory $service.Dir -PassThru
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
