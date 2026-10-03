Stop-Process -Name nginx -Force -ErrorAction SilentlyContinue
Write-Host "Nginx stopped." -ForegroundColor Yellow
