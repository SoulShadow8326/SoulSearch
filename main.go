package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	var mode = flag.String("mode", "server", "Mode: server, proxy, crawl, or index")
	var port = flag.Int("port", 8080, "Proxy port")
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
		server := NewServer(0)
		log.Println("Starting SoulSearch Unix socket server")
		server.Start()
	case "proxy":
		proxy := NewProxy(*port, "/tmp/soulsearch.sock")
		proxy.Start()
	default:
		fmt.Println("Invalid mode. Use: server, proxy, crawl, or index")
		os.Exit(1)
	}
}
