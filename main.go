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
	flag.Parse()

	switch *mode {
	case "crawl":
		if *url == "" {
			fmt.Println("URL required for crawling mode")
			os.Exit(1)
		}
		crawler := NewContentCrawler()
		_ = crawler
	case "index":
		indexer := NewIndexer()
		indexer.BuildIndex()
	case "server":
		server := NewServer(*port)
		log.Printf("Starting SoulSearch HTTP server on port %d", *port)
		server.Start()
	case "proxy":
		proxy := NewProxy(*port, "/tmp/exsearch.sock")
		proxy.Start()
	default:
		fmt.Println("Invalid mode. Use: server, proxy, crawl, or index")
		os.Exit(1)
	}
}
