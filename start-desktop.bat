@echo off
cd /d "%~dp0"
start /b server.exe
timeout /t 2 /nobreak >nul
powershell -ExecutionPolicy Bypass -File ".video-downloader-window.ps1"
