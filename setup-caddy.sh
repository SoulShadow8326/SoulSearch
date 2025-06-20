#!/bin/bash

echo "Setting up SoulSearch for Hack Club Nest..."

# Ensure we're in the right directory
cd ~/pub

# Build the frontend if needed
if [ -d "frontend" ] && [ ! -d "frontend/build" ]; then
    echo "Building frontend..."
    cd frontend
    npm install
    npm run build
    cd ..
fi

# Build the Go binary
echo "Building SoulSearch..."
go build -o soulsearch

# Copy the Caddyfile to the home directory
echo "Setting up Caddyfile..."
cp Caddyfile ~/Caddyfile

# Reload Caddy
echo "Reloading Caddy..."
systemctl --user reload caddy

# Start SoulSearch
echo "Starting SoulSearch on port 8080..."
./run.sh --mode=full --port=8080 &

echo "Setup complete!"
echo "Your site should be available at: https://soul.hackclub.app"
echo "API endpoint: https://soul.hackclub.app/api/dynamic-search"
