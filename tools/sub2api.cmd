@echo off
REM Project-local command entry for the Sub2API launcher.
pwsh.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0sub2api.ps1"
exit /b %ERRORLEVEL%
