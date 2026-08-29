@echo off
REM SHEYTAN(TM)-Local-Agent v1.0.11 launcher
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
REM v1.0.9: TURBINE — smooth 120fps streaming (frame-paced pump with a
REM live tok/s readout), the complete file studio (combine, search,
REM replace, chunked reads), and a faster data engine: zero-copy CSV
REM parsing, parse-once numeric caches and new analysis actions
REM (regression, outliers, pivot, moving average and more).
REM v1.0.10: PRISM — the build error that broke v1.0.9 on GitHub is fixed
REM (internal/sessions + internal/sandbox now ship in the source tree),
REM four new agent tools (json query/transform, archive zip/tar, fetch
REM URL reader, diff verifier), the activity feed moved to an append-only
REM sidecar (serve mode no longer rewrites whole session files per tool
REM event) and the recall engine caches its BM25 corpus stats.
REM v1.0.11: GRANITE — the release that actually builds on GitHub. Root
REM cause of the v1.0.9/v1.0.10 CI failures found: an UNANCHORED .gitignore
REM pattern silently swallowed internal/sessions + internal/sandbox from
REM every commit (patterns are now root-anchored + a release-gate test
REM guards it in CI). Memory IDs are collision-proof on Windows clock
REM granularity, TrimLogs no longer renames over an open file (it freed
REM zero bytes on Windows), and CI pins Go 1.26 with repaired branch
REM triggers and Node-24 actions.

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
