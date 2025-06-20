#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
MODE="full"
WORKERS=8
PORT=8080
SOCK_PATH="/tmp/soulsearch.sock"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode=*)
            MODE="${1#*=}"
            shift
            ;;
        --workers=*)
            WORKERS="${1#*=}"
            shift
            ;;
        --port=*)
            PORT="${1#*=}"
            shift
            ;;
        --mode)
            MODE="$2"
            shift 2
            ;;
        --workers)
            WORKERS="$2"
            shift 2
            ;;
        --port)
            PORT="$2"
            shift 2
            ;;
        --help|-h)
            echo "SoulSearch Enterprise Launcher"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "OPTIONS:"
            echo "  --mode MODE       Run mode: full, crawler, server, client (default: full)"
            echo "  --workers NUM     Number of crawler workers (default: 8)"
            echo "  --port PORT       Server port (default: 8080)"
            echo "  --help, -h        Show this help message"
            echo ""
            echo "MODES:"
            echo "  full     - Start both distributed crawler and search server"
            echo "  crawler  - Start only the distributed crawler"
            echo "  server   - Start only the search server"
            echo "  client   - Interactive client to manage crawler"
            echo ""
            echo "EXAMPLES:"
            echo "  $0                           # Start full system"
            echo "  $0 --mode=crawler --workers=16  # Start crawler with 16 workers"
            echo "  $0 --mode=server --port=9000     # Start server on port 9000"
            echo "  $0 --mode=client                 # Interactive client mode"
            exit 0
            ;;
        *)
            echo "Unknown option $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}🚀 SoulSearch Enterprise Search Engine${NC}"
echo -e "${BLUE}======================================${NC}"

# Cleanup any existing processes
echo -e "${YELLOW}Cleaning up existing processes...${NC}"
pkill -f "soulsearch.*distributed" 2>/dev/null || true
pkill -f "soulsearch.*server" 2>/dev/null || true
rm -f "$SOCK_PATH" 2>/dev/null || true

# Build the application
echo -e "${YELLOW}Building SoulSearch...${NC}"
if [ -d "frontend" ]; then
    echo -e "${YELLOW}Building frontend...${NC}"
    cd frontend
    npm install --silent
    npm run build --silent
    cd ..
fi

go build -o soulsearch

echo -e "${GREEN}✅ Build complete${NC}"

# Function to start distributed crawler
start_crawler() {
    echo -e "${YELLOW}Starting distributed crawler with $WORKERS workers...${NC}"
    ./soulsearch -mode=distributed -workers=$WORKERS -sock="$SOCK_PATH" &
    CRAWLER_PID=$!
    
    # Wait for crawler to start
    sleep 3
    
    if kill -0 $CRAWLER_PID 2>/dev/null; then
        echo -e "${GREEN}✅ Distributed crawler running (PID: $CRAWLER_PID)${NC}"
        echo -e "${BLUE}   Socket: $SOCK_PATH${NC}"
        echo -e "${BLUE}   Workers: $WORKERS${NC}"
    else
        echo -e "${RED}❌ Failed to start crawler${NC}"
        exit 1
    fi
}

# Function to start search server
start_server() {
    echo -e "${YELLOW}Starting search server on port $PORT...${NC}"
    ./soulsearch -mode=server -port=$PORT &
    SERVER_PID=$!
    
    # Wait for server to start
    sleep 2
    
    if kill -0 $SERVER_PID 2>/dev/null; then
        echo -e "${GREEN}✅ Search server running (PID: $SERVER_PID)${NC}"
        echo -e "${BLUE}   API: http://localhost:$PORT/api/dynamic-search${NC}"
        echo -e "${BLUE}   Web UI: http://localhost:$PORT/${NC}"
    else
        echo -e "${RED}❌ Failed to start server${NC}"
        exit 1
    fi
}

# Function to show crawler stats
show_stats() {
    if [ -S "$SOCK_PATH" ]; then
        echo -e "${BLUE}📊 Crawler Statistics:${NC}"
        ./soulsearch -mode=client -sock="$SOCK_PATH" -cmd=stats
    else
        echo -e "${RED}❌ Crawler not running${NC}"
    fi
}

# Function for interactive client
interactive_client() {
    if [ ! -S "$SOCK_PATH" ]; then
        echo -e "${RED}❌ Crawler not running. Start with --mode=crawler or --mode=full first.${NC}"
        exit 1
    fi
    
    echo -e "${BLUE}🎮 Interactive Crawler Client${NC}"
    echo -e "${BLUE}=============================${NC}"
    
    while true; do
        echo ""
        echo "Commands:"
        echo "  1) Add single URL"
        echo "  2) Add bulk URLs"
        echo "  3) Show statistics"
        echo "  4) Exit"
        echo ""
        read -p "Choose an option [1-4]: " choice
        
        case $choice in
            1)
                read -p "Enter URL: " url
                if [ -n "$url" ]; then
                    ./soulsearch -mode=client -sock="$SOCK_PATH" -cmd=add -url="$url"
                fi
                ;;
            2)
                read -p "Enter URLs (comma-separated): " urls
                if [ -n "$urls" ]; then
                    ./soulsearch -mode=client -sock="$SOCK_PATH" -cmd=bulk -urls="$urls"
                fi
                ;;
            3)
                show_stats
                ;;
            4)
                echo -e "${YELLOW}Goodbye!${NC}"
                exit 0
                ;;
            *)
                echo -e "${RED}Invalid option${NC}"
                ;;
        esac
    done
}

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}🛑 Shutting down SoulSearch...${NC}"
    
    if [ -n "$SERVER_PID" ]; then
        kill $SERVER_PID 2>/dev/null || true
        echo -e "${GREEN}✅ Search server stopped${NC}"
    fi
    
    if [ -n "$CRAWLER_PID" ]; then
        kill $CRAWLER_PID 2>/dev/null || true
        echo -e "${GREEN}✅ Distributed crawler stopped${NC}"
    fi
    
    rm -f "$SOCK_PATH" 2>/dev/null || true
    echo -e "${GREEN}✅ Cleanup complete${NC}"
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM

# Main execution based on mode
case $MODE in
    "full")
        start_crawler
        start_server
        echo ""
        echo -e "${GREEN}🎉 SoulSearch Enterprise is running!${NC}"
        echo -e "${BLUE}📊 Real-time stats: $0 --mode=client${NC}"
        echo -e "${BLUE}🔍 Search: curl 'http://localhost:$PORT/api/dynamic-search?q=test'${NC}"
        echo ""
        echo -e "${YELLOW}Press Ctrl+C to stop all services${NC}"
        
        # Show initial stats after a moment
        sleep 5
        show_stats
        
        # Keep running
        wait
        ;;
    "crawler")
        start_crawler
        echo ""
        echo -e "${GREEN}🎉 Distributed crawler is running!${NC}"
        echo -e "${BLUE}📊 Stats: $0 --mode=client${NC}"
        echo ""
        echo -e "${YELLOW}Press Ctrl+C to stop crawler${NC}"
        
        # Show stats every 30 seconds
        while kill -0 $CRAWLER_PID 2>/dev/null; do
            sleep 30
            show_stats
        done
        ;;
    "server")
        start_server
        echo ""
        echo -e "${GREEN}🎉 Search server is running!${NC}"
        echo -e "${BLUE}🔍 Test: curl 'http://localhost:$PORT/api/dynamic-search?q=test'${NC}"
        echo ""
        echo -e "${YELLOW}Press Ctrl+C to stop server${NC}"
        
        wait
        ;;
    "client")
        interactive_client
        ;;
    *)
        echo -e "${RED}❌ Unknown mode: $MODE${NC}"
        echo "Use --help for available modes"
        exit 1
        ;;
esac
