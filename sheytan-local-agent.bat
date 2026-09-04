@echo off
REM SHEYTAN(TM)-Local-Agent v1.1.2 (Zeta) launcher
REM
REM Double-click to launch the native desktop GUI.
REM (c) 2024-2026 Parsaetak. All rights reserved.
REM SHEYTAN is a trademark of Parsaetak (https://github.com/Parsaetak).
REM Everything (models, sessions, logs, charts) lives in this folder - portable.
REM
REM v1.1.2Z (Zeta): the AAA-completion release. The Node/React UI now ships
REM a 120Hz-first motion system (compositor-only animations, spring easing,
REM staggered entrances, reduced-motion support). CI pins Node 24 + Go 1.26,
REM restores the gen-syso icon/version-info pipeline and the
REM -H=windowsgui subsystem flag, runs the stress gate in CI, and ships this
REM .bat launcher inside the portable ZIP. CI-produced releases:
REM SHEYTAN-Local-Agent-Windows-x64-v1.1.2Z.zip and the Linux x64 twin.

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
