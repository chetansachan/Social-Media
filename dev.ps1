# dev.ps1 - Start all services with hot-reloading using Air
$ErrorActionPreference = "Continue"

Write-Host "Starting microservices with Air..." -ForegroundColor Cyan

# Option A: If npx concurrently is available, use it for unified color-coded logs in one terminal
if (Get-Command "npx" -ErrorAction SilentlyContinue) {
    Write-Host "Detected npx. Running via concurrently for combined logs..." -ForegroundColor Green
    npx concurrently -n "USER-SVC,POST-SVC,FOLLOW-SVC,FEED-SVC,GATEWAY" -c "cyan,blue,yellow,green,magenta" `
        "cd user-service && air" `
        "cd post-service && air" `
        "cd follow-service && air" `
        "cd feed-service && air" `
        "cd api-gateway && air"
    exit
}

# Option B: Fallback to PowerShell background processes
Write-Host "Running services in separate Air processes..." -ForegroundColor Yellow

$p1 = Start-Process -FilePath "air" -WorkingDirectory ".\user-service" -PassThru
$p2 = Start-Process -FilePath "air" -WorkingDirectory ".\post-service" -PassThru
$p3 = Start-Process -FilePath "air" -WorkingDirectory ".\follow-service" -PassThru
$p4 = Start-Process -FilePath "air" -WorkingDirectory ".\feed-service" -PassThru
$p5 = Start-Process -FilePath "air" -WorkingDirectory ".\api-gateway" -PassThru

Write-Host "User Service   PID: $($p1.Id)" -ForegroundColor Cyan
Write-Host "Post Service   PID: $($p2.Id)" -ForegroundColor Blue
Write-Host "Follow Service PID: $($p3.Id)" -ForegroundColor Yellow
Write-Host "Feed Service   PID: $($p4.Id)" -ForegroundColor Green
Write-Host "API Gateway    PID: $($p5.Id)" -ForegroundColor Magenta
Write-Host "Press Ctrl+C to stop all services..." -ForegroundColor White

try {
    Wait-Process -Id $p1.Id, $p2.Id, $p3.Id, $p4.Id, $p5.Id
}
finally {
    Write-Host "`nStopping all services..." -ForegroundColor Red
    Stop-Process -Id $p1.Id, $p2.Id, $p3.Id, $p4.Id, $p5.Id -ErrorAction SilentlyContinue
}
