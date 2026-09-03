@echo off
REM SHEYTAN(TM)-Local-Agent v1.1.1 (Zeta) launcher
REM
REM Double-click to launch the native desktop GUI.
REM (c) 2024-2026 Parsaetak. All rights reserved.
REM SHEYTAN is a trademark of Parsaetak (https://github.com/Parsaetak).
REM Everything (models, sessions, logs, charts) lives in this folder - portable.
REM
REM v1.1.1Z (Zeta): the Node/React UI release. The legacy Fyne UI is gone;
REM the app ships the Vite-built React frontend embedded in the exe. CI pins
REM Node 24 + Go 1.26, installs the correct GTK4/WebKitGTK-6.0 headers for
REM Linux builds, and the Windows exe carries the brand icon, Parsa Tak
REM signature and DPI-aware manifest. CI-produced releases:
REM SHEYTAN-Local-Agent-Windows-x64-v1.1.1Z.zip and the Linux x64 twin.

setlocal

set "SCRIPT_DIR=%~dp0"
cd /d "%SCRIPT_DIR%"

if exist "%SCRIPT_DIR%sheytan-local-agent.exe" (
    start "" "%SCRIPT_DIR%sheytan-local-agent.exe" %*
    goto :done
)

where sheytan-local-agent.exe >nul 2>&1
if %errorlevel%==0 (
    start "" sheytan-local-agent.exe %*
    goto :done
)

echo sheytan-local-agent.exe was not found next to this launcher.
echo Extract the full zip and run it from the sheytan-local-agent folder.
pause

:done
endlocal
