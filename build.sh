#!/bin/bash
echo "Building Pool League Manager..."
go mod tidy
go build -ldflags="-s -w" -o league_app .
echo "Done! Run ./league_app to start."
