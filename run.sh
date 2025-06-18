#!/bin/bash

echo "Starting SoulSearch..."

# Kill any existing processes
pkill -f "go run.*main.go" 2>/dev/null
pkill -f "./soulsearch" 2>/dev/null

# Build frontend
echo "Building frontend..."
cd frontend
npm start
cd ..

# Build Go backend
echo "Building backend..."
go build -o soulsearch *.go
if [ $? -ne 0 ]; then
    echo "Backend build failed!"
    exit 1
fi

# Start the server (it serves both API and static files)
echo "Starting SoulSearch server on http://localhost:8080..."
./soulsearch -mode=server -port=8080 &
SERVER_PID=$!

sleep 2

echo "SoulSearch is running!"
echo "API available at: http://localhost:8080/api/"
echo "Web UI available at: http://localhost:8080/"
echo ""
echo "Press Ctrl+C to stop the server"

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "Stopping SoulSearch..."
    kill $SERVER_PID 2>/dev/null
    echo "Server stopped"
    exit 0
}

# Set trap to cleanup on script exit
trap cleanup SIGINT SIGTERM

# Wait for the server process
wait
