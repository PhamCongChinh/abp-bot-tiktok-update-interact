@echo off
go build -o bot.exe cmd/main.go
if %errorlevel% neq 0 (
    echo Build failed.
    exit /b %errorlevel%
)
echo Build succeeded: bot.exe
