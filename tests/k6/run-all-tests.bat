@echo off
REM ======================================================
REM GopherStack Enterprise — k6 Test Runner for Windows
REM ======================================================
REM
REM Requirements:
REM   1. Install k6:  winget install k6  (or https://k6.io/docs/get-started/installation/)
REM   2. Start GopherStack:  gopherstack.exe start
REM   3. Run this script
REM ======================================================

setlocal enabledelayedexpansion

cd /d "%~dp0.."

:: Config
set BASE_URL=http://localhost:80
set DASHBOARD_URL=http://localhost:8090
set K6_OPTS=--env BASE_URL=%BASE_URL% --env DASHBOARD_URL=%DASHBOARD_URL%
:: If you want HTML report, install xk6-output-format-html or use k6 v0.54+ built-in
set K6_OUT=

if not exist "gopherstack.exe" (
    echo [ERROR] gopherstack.exe not found!
    echo         Make sure you're in the project root directory.
    echo         Current: %CD%
    exit /b 1
)

echo ╔══════════════════════════════════════════════╗
echo ║   GopherStack Enterprise — k6 Test Suite     ║
echo ╚══════════════════════════════════════════════╝
echo.
echo Target:  %BASE_URL%
echo Dashboard: %DASHBOARD_URL%
echo.
echo Pastikan server SUDAH jalan: gopherstack.exe start
echo.
choice /M "Lanjutkan testing"
if errorlevel 2 exit /b 0

echo.
echo ============================================================
echo [1/4] SMOKE TEST — Basic functionality check
echo ============================================================
k6 run tests/k6/smoke-test.js %K6_OPTS%
if errorlevel 1 (
    echo [WARN] Smoke test gagal! Lanjut? 
    pause
)

echo.
echo ============================================================
echo [2/4] LOAD TEST — Ramp up 10-&gt;50 concurrent users
echo ============================================================
k6 run tests/k6/load-test.js %K6_OPTS%
if errorlevel 1 (
    echo [WARN] Load test ada error!
    pause
)

echo.
echo ============================================================
echo [3/4] STRESS TEST — 10 -&gt; 500 users, cari breaking point
echo ============================================================
k6 run tests/k6/stress-test.js %K6_OPTS%
if errorlevel 1 (
    echo [WARN] Stress test ada error!
    pause
)

echo.
echo ============================================================
echo [4/4] SPIKE TEST — Lonjakan traffic mendadak
echo ============================================================
k6 run tests/k6/spike-test.js %K6_OPTS%
if errorlevel 1 (
    echo [WARN] Spike test ada error!
    pause
)

echo.
echo ============================================================
echo DASHBOARD API TEST
echo ============================================================
k6 run tests/k6/dashboard-test.js %K6_OPTS%

echo.
echo ============================================================
echo ✅ SEMUA TEST SELESAI!
echo ============================================================
echo.
echo Review hasil di atas, atau jalankan individual:
echo   k6 run tests/k6/smoke-test.js
echo   k6 run tests/k6/stress-test.js
echo   k6 run tests/k6/full-suite.js --out json=results.json
echo.
pause
