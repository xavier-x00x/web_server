# GopherStack - Stop all processes script
# This script kills all GopherStack related processes to ensure a clean state.

Write-Host "Stopping GopherStack processes..." -ForegroundColor Cyan

# Kill GopherStack main process
$gs = Get-Process gopherstack -ErrorAction SilentlyContinue
if ($gs) {
    Write-Host "Terminating gopherstack.exe (PID: $($gs.Id))..."
    Stop-Process -Id $gs.Id -Force
}

# Kill Nginx
$ngx = Get-Process nginx, gopher-nginx -ErrorAction SilentlyContinue
if ($ngx) {
    Write-Host "Terminating Nginx processes..."
    Stop-Process -Name nginx, gopher-nginx -Force
}

# Kill PHP workers
$php = Get-Process gopher-php -ErrorAction SilentlyContinue
if ($php) {
    Write-Host "Terminating $($php.Count) PHP worker processes..."
    Stop-Process -Name gopher-php -Force
}

Write-Host "All processes terminated. Port 8090 and 8088 should be free now." -ForegroundColor Green
