@echo off
setlocal
where pwsh.exe >nul 2>nul
if %ERRORLEVEL% EQU 0 goto pwsh
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "%~dp0healthsync.ps1" %*
exit /b %ERRORLEVEL%
:pwsh
pwsh.exe -NoLogo -NoProfile -NonInteractive -File "%~dp0healthsync.ps1" %*
exit /b %ERRORLEVEL%
