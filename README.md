# SoulSearch

A high-performance search engine built from scratch in pure Go. Features web crawling, custom indexing, PageRank algorithm, and a modern web interface.

## Features

- **Web Crawler**: Multi-threaded crawler with polite crawling delays
- **Custom Indexer**: Term frequency analysis with stop word filtering
- **PageRank Algorithm**: Link-based authority scoring
- **Advanced Scoring**: Combines TF-IDF, PageRank, and position-based scoring
- **Modern Web UI**: Responsive interface with real-time search
- **Pure Go**: No external dependencies, built with standard library only

## Usage

### 1. Build the application
```bash
go build -o SoulSearch
```

### 2. Crawl websites
```bash
./SoulSearch -mode=crawl -url="https://example.com" -max=1000
```

### 3. Build the search index
```bash
./SoulSearch -mode=index
```

### 4. Start the search server
```bash
./SoulSearch -mode=server -port=8080
```

### 5. Search via web interface
Open http://localhost:8080 in your browser

### 6. Search via API
```bash
curl "http://localhost:8080/api/search?q=your+query&max=10"
```


## Scoring Algorithm

1. **Term Frequency (TF)**: How often query terms appear
2. **PageRank**: Link-based authority score
3. **Position Boost**: Higher weight for title matches
4. **Phrase Matching**: Exact phrase gets 2x boost
6. **Query Coverage**: Percentage of query terms matched

## Performance

- **Crawling**: ~10 pages/second with respectful delays
- **Indexing**: Processes 1000 documents in ~500ms
- **Memory**: Efficient inverted index structure

## Development

The codebase is organized into focused modules:

- `main.go`: Entry point and CLI handling
- `crawler.go`: Web crawling engine
- `indexer.go`: Document indexing and PageRank
- `search.go`: Search algorithm and ranking

## Extending

To add new features:

1. **Custom Ranking**: Modify `calculateScore()` in `search.go`
2. **New Data Sources**: Extend crawler in `crawler.go`
4. **UI Enhancements**: Update HTML templates in `handleHome()`

## License

MIT License - Build amazing things!
