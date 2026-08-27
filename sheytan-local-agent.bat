@echo off
REM SHEYTAN(TM)-Local-Agent v1.0.8 launcher
REM
REM Double-click to launch the native desktop GUI.
REM (c) 2024-2026 Parsaetak. All rights reserved.
REM SHEYTAN is a trademark of Parsaetak (https://github.com/Parsaetak).
REM Everything (models, sessions, logs, charts) lives in this folder - portable.
REM
REM v1.0.4: the GUI is started DETACHED ("start"), so this console closes
REM itself immediately instead of sitting open for the whole session. The
REM .exe itself is a GUI app (no console) — you can also double-click it
REM directly. v1.0.5: the .exe now carries the app icon, version info, and
REM a DPI-aware manifest (crisp UI on scaled displays). v1.0.6: VISION —
REM drop an mmproj-*.gguf next to your model and the agent can see images
REM and screenshots; plus a built-in Terminal, Resources panel and Ctrl+K.
REM v1.0.7: CONTINUUM — almost unlimited context via chapter rollover
REM (live context meter above the composer), plus the Ember Luxe UI
REM redesign (gradient buttons, glass surfaces, glow rings).
REM v1.0.8: AURORA — attachment crash fixed (native Win32 picker + panic
REM guards), the unified pill composer, Aurora gradient buttons, modern
REM icons, signed under the name "Parsa Tak", and faster on Windows
REM (GC tuned for streaming, direct syscalls, icon caches).

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
