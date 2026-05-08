Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " GopherStack Enterprise - K6 Stress Test Runner" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# Check if k6 is installed
if (!(Get-Command k6 -ErrorAction SilentlyContinue)) {
    Write-Host "K6 is not installed on your system." -ForegroundColor Yellow
    Write-Host "Attempting to install K6 via winget..." -ForegroundColor Cyan
    winget install k6 --source winget --accept-package-agreements --accept-source-agreements
    
    # Reload environment to pick up path changes if necessary
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    
    if (!(Get-Command k6 -ErrorAction SilentlyContinue)) {
        Write-Host "Failed to install K6 automatically (or needs terminal restart)." -ForegroundColor Red
        Write-Host "Please download it from https://k6.io/docs/get-started/installation/"
        Write-Host "Or install manually using: winget install k6"
        exit 1
    }
}

Write-Host "`nRunning K6 Test Script (k6_test.js)...`n" -ForegroundColor Green
k6 run k6_test.js

Write-Host "`nDone!" -ForegroundColor Cyan
