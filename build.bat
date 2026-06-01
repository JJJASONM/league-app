@echo off
echo Building Pool League Manager for Windows...
go mod tidy
go build -ldflags="-s -w" -o league_app.exe .
echo.
echo Done! Run league_app.exe to start.
pause
