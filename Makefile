BINARY_NAME=soulsearch
PORT=8080

.PHONY: build clean run-server run-proxy crawl-demo index dev help

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)
	rm -rf data/
	rm -f /tmp/soulsearch.sock

run-server: build
	./$(BINARY_NAME) -mode=server

run-proxy: build
	./$(BINARY_NAME) -mode=proxy -port=$(PORT)

dev: build
	@echo "Starting Unix socket server in background..."
	./$(BINARY_NAME) -mode=server &
	@sleep 2
	@echo "Starting HTTP proxy on port $(PORT)..."
	./$(BINARY_NAME) -mode=proxy -port=$(PORT)

crawl-demo: build
	./$(BINARY_NAME) -mode=crawl -url="https://news.ycombinator.com" -max=100

index: build
	./$(BINARY_NAME) -mode=index

full-demo: build
	@echo "Step 1: Crawling demo site..."
	./$(BINARY_NAME) -mode=crawl -url="https://news.ycombinator.com" -max=50
	@echo "Step 2: Building search index..."
	./$(BINARY_NAME) -mode=index
	@echo "Step 3: Starting Unix socket server in background..."
	./$(BINARY_NAME) -mode=server &
	@sleep 2
	@echo "Step 4: Starting HTTP proxy on port $(PORT)..."
	./$(BINARY_NAME) -mode=proxy -port=$(PORT)

help:
	@echo "SoulSearch Build Commands:"
	@echo "  build      - Compile the search engine"
	@echo "  clean      - Remove binary and data files"
	@echo "  run-server - Start the web server"
	@echo "  crawl-demo - Crawl Hacker News for demo"
	@echo "  index      - Build search index from crawled data"
	@echo "  full-demo  - Complete demo: crawl + index + serve"
	@echo "  help       - Show this help message"
