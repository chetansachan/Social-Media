$nginxDir = "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\nginxinc.nginx_Microsoft.Winget.Source_8wekyb3d8bbwe\nginx-1.31.6"
$nginxExe = Join-Path $nginxDir "nginx.exe"
$conf = Join-Path $PSScriptRoot "nginx.conf"

if (-not (Test-Path $nginxExe)) {
    # Fallback to PATH
    $cmd = Get-Command nginx -ErrorAction SilentlyContinue
    if ($cmd) {
        $nginxExe = $cmd.Source
        $nginxDir = Split-Path $nginxExe
    } else {
        Write-Error "nginx.exe not found! Please check your installation."
        exit 1
    }
}

# Stop any running instances first to prevent port 80 conflicts
Stop-Process -Name nginx -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500

# Test config
Write-Host "Testing configuration..." -ForegroundColor Cyan
& $nginxExe -p $nginxDir -c $conf -t
if ($LASTEXITCODE -ne 0) {
    Write-Error "Nginx configuration test failed!"
    exit 1
}

# Start Nginx
Write-Host "Starting Nginx..." -ForegroundColor Cyan
Start-Process -FilePath $nginxExe -ArgumentList "-p `"$nginxDir`" -c `"$conf`"" -WindowStyle Hidden

Start-Sleep -Seconds 1
$running = Get-Process nginx -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "SUCCESS: Nginx is running and listening on http://localhost" -ForegroundColor Green
    Write-Host "Reverse proxy -> http://127.0.0.1:8080" -ForegroundColor Green
} else {
    Write-Error "Nginx failed to stay running. Check logs in $nginxDir\logs\error.log"
}
