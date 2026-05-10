@echo off
setlocal EnableDelayedExpansion

echo ===========================================
echo  FpbxCTC Installer Build
echo ===========================================
echo.

:: Run from the directory containing this script
cd /d "%~dp0"

:: Always prepend Go bin dir so it is found even in fresh terminals
set PATH=C:\Program Files\Go\bin;!PATH!

:: Always prepend Cargo bin dir so cargo/installrs are found
set PATH=!USERPROFILE!\.cargo\bin;!PATH!

:: Verify Go
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Go not found.  Install from https://go.dev/dl/
    goto :fail
)

:: Verify installrs CLI
where installrs >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: 'installrs' CLI not found.
    echo.
    echo Install it with:
    echo   cargo install installrs --locked
    echo.
    echo Requires the Rust toolchain: https://rustup.rs/
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

echo.
echo [4/6] Building FpbxCTC.exe ...
go build -ldflags "-H windowsgui -s -w" -o FpbxCTC.exe .
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Go build failed.
    goto :fail
)

echo.
echo [5/6] Copying files into installer folder ...
copy /Y FpbxCTC.exe installer\FpbxCTC.exe >nul
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Could not copy FpbxCTC.exe to installer\
    goto :fail
)
copy /Y FpbxCTC.ico installer\FpbxCTC.ico >nul
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Could not copy FpbxCTC.ico to installer\
    goto :fail
)

echo.
echo [6/6] Building FpbxCTC-Setup.exe via installrs ...
installrs build installer --output FpbxCTC-Setup.exe
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: installrs build failed.
    goto :fail
)

echo.
echo ===========================================
echo  Done!  FpbxCTC-Setup.exe is ready.
echo ===========================================
echo.
echo Distribute FpbxCTC-Setup.exe.
echo Running it will:
echo   - Install FpbxCTC to C:\Program Files\FpbxCTC\
echo   - Add a Start Menu shortcut
echo   - Add an entry to Add/Remove Programs
echo   - Place an uninstaller alongside the app
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
