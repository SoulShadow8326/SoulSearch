#!/bin/bash

set -e

echo "Starting SoulSearch..."

pkill -f "go run.*main.go" 2>/dev/null || true
pkill -f "./soulsearch" 2>/dev/null || true

cd frontend
npm install
npm run build
cd ..

go build -o soulsearch *.go

./soulsearch -mode=server -port=8080 &
SERVER_PID=$!

sleep 2

echo "SoulSearch is running!"
echo "API available at: http://localhost:8080/api/"
echo "Web UI available at: http://localhost:8080/"
echo ""
echo "Press Ctrl+C to stop the server"

cleanup() {
    echo ""
    echo "Stopping SoulSearch..."
    kill $SERVER_PID 2>/dev/null || true
    echo "Server stopped"
    exit 0
}

trap cleanup SIGINT SIGTERM

wait
