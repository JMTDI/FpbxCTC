@echo off
setlocal EnableDelayedExpansion

echo ===========================================
echo  FpbxCTC Build Script
echo ===========================================
echo.

:: Run from the directory containing this script
cd /d "%~dp0"

:: Always prepend Go bin dir
set PATH=C:\Program Files\Go\bin;!PATH!

:: Verify Go is available
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: 'go' not found. Install Go from https://go.dev/dl/
    goto :fail
)

:: ── Step 1: Convert FpbxCTC.png → FpbxCTC.ico ───────────────────────────────
echo [1/6] Converting icon (PNG ^> ICO)...
go run ./tools/mkico FpbxCTC.png FpbxCTC.ico
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Icon conversion failed.
    goto :fail
)

:: ── Step 2: Generate browser extension icons ─────────────────────────────────
echo.
echo [2/6] Generating browser extension icons...
go run ./tools/mkicons FpbxCTC.png browser-extension\icons
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Browser icon generation failed.
    goto :fail
)

:: ── Step 3: Embed icon into EXE (creates rsrc.syso picked up by go build) ───
echo.
echo [3/6] Embedding icon in EXE...
go run github.com/akavel/rsrc@latest -ico FpbxCTC.ico -o rsrc.syso
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: rsrc failed.
    goto :fail
)

:: ── Step 4: Tidy dependencies ────────────────────────────────────────────────
echo.
echo [4/6] Tidying dependencies...
go mod tidy
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo ERROR: 'go mod tidy' failed.
    goto :fail
)

echo.
echo [5/6] Building FpbxCTC.exe ...
go build -ldflags "-H windowsgui -s -w" -o FpbxCTC.exe .
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo ERROR: Build failed. See output above.
    goto :fail
)

echo.
echo [6/6] Done.
echo.
echo   Output : FpbxCTC.exe
echo.
echo Usage:
echo   FpbxCTC.exe              -- open Settings window
echo   FpbxCTC.exe "tel:1234"   -- dial 1234 via the API (used by Windows)
echo.
echo In the Settings window:
echo   1. Fill in Domain, API Key, Agent Number and click Save Settings.
echo   2. Click "Register as tel: handler".
echo   3. On Windows 11 click "Open Windows Default Apps" and select FpbxCTC.
echo.
goto :end

:fail
echo.
echo Build FAILED.
pause
exit /b 1

:end
pause
endlocal
