package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	var mode = flag.String("mode", "server", "Mode: server, crawl, or index")
	var port = flag.Int("port", 8080, "Server port")
	var url = flag.String("url", "", "URL to start crawling from")
	var maxPages = flag.Int("max", 1000, "Maximum pages to crawl")
	flag.Parse()

	switch *mode {
	case "crawl":
		if *url == "" {
			fmt.Println("URL required for crawling mode")
			os.Exit(1)
		}
		crawler := NewCrawler(*maxPages)
		crawler.CrawlFromSeed(*url)
	case "index":
		indexer := NewIndexer()
		indexer.BuildIndex()
	case "server":
		server := NewServer(*port)
		log.Printf("Starting SoulSearch server on port %d", *port)
		server.Start()
	default:
		fmt.Println("Invalid mode. Use: server, crawl, or index")
		os.Exit(1)
	}
}
