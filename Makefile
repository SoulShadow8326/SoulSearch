BINARY_NAME=soulsearch
PORT=8080

.PHONY: build clean run-server crawl-demo index help

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)
	rm -rf data/

run-server: build
	./$(BINARY_NAME) -mode=server -port=$(PORT)

crawl-demo: build
	./$(BINARY_NAME) -mode=crawl -url="https://news.ycombinator.com" -max=100

index: build
	./$(BINARY_NAME) -mode=index

full-demo: build
	@echo "Step 1: Crawling demo site..."
	./$(BINARY_NAME) -mode=crawl -url="https://news.ycombinator.com" -max=50
	@echo "Step 2: Building search index..."
	./$(BINARY_NAME) -mode=index
	@echo "Step 3: Starting search server on port $(PORT)..."
	./$(BINARY_NAME) -mode=server -port=$(PORT)

help:
	@echo "SoulSearch Build Commands:"
	@echo "  build      - Compile the search engine"
	@echo "  clean      - Remove binary and data files"
	@echo "  run-server - Start the web server"
	@echo "  crawl-demo - Crawl Hacker News for demo"
	@echo "  index      - Build search index from crawled data"
	@echo "  full-demo  - Complete demo: crawl + index + serve"
	@echo "  help       - Show this help message"
