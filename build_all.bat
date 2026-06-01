@echo off
echo Building for all platforms...
go mod tidy

echo Building Windows...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o dist\league_app_windows.exe .

echo Building macOS (Intel)...
set GOOS=darwin
set GOARCH=amd64
go build -ldflags="-s -w" -o dist\league_app_macos_intel .

echo Building macOS (Apple Silicon)...
set GOOS=darwin
set GOARCH=arm64
go build -ldflags="-s -w" -o dist\league_app_macos_arm64 .

echo.
echo All builds in dist\
pause
