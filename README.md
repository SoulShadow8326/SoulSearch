# SoulSearch

SoulSearch is a semantic web search engine written in pure Go. It performs polite, concurrent crawling of the web, builds an inverted index, computes PageRank periodically, and exposes HTTP endpoints for semantic search.

## Features

- **Web Crawler**: Multi-threaded crawler with polite crawling delays
- **Custom Indexer**: Term frequency analysis with stop word filtering
- **PageRank Algorithm**: Link-based authority scoring
- **Advanced Scoring**: Combines TF-IDF, PageRank, and position-based scoring
- **Pure Go**: No external dependencies, built with standard library only

## Usage


## Running the Backend

### Requirements

- Go 1.20 or higher
- SQLite3
- Node.js (for frontend)
- Git

```bash
git clone https://github.com/SoulShadow8326/soulsearch
cd soulsearch
go run .
```
## running the frontend
```bash
cd frontend
npm install
npm run dev
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
