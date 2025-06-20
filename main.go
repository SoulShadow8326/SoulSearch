package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	var mode = flag.String("mode", "server", "Mode: server, proxy, crawl, index, distributed, or client")
	var port = flag.Int("port", 8080, "Server/proxy port")
	var url = flag.String("url", "", "URL to start crawling from")
	var workers = flag.Int("workers", 0, "Number of crawler workers (0 = auto)")
	var sockPath = flag.String("sock", "/tmp/soulsearch.sock", "Unix socket path for distributed crawler")
	var command = flag.String("cmd", "", "Client command: add, bulk, stats")
	var urls = flag.String("urls", "", "Comma-separated URLs for bulk add")
	flag.Parse()

	switch *mode {
	case "crawl":
		if *url == "" {
			fmt.Println("URL required for crawling mode")
			os.Exit(1)
		}
		crawler := CreateContentCrawler()
		_ = crawler
	case "distributed":
		crawler := CreateDistributedCrawler(*workers, *sockPath)
		if err := crawler.Start(); err != nil {
			fmt.Printf("Failed to start distributed crawler: %v\n", err)
			os.Exit(1)
		}

		select {}
	case "client":
		if *command == "" {
			fmt.Println("Command required for client mode. Use: add, bulk, stats")
			os.Exit(1)
		}

		client := CreateCrawlerClientCmd(*sockPath)
		conn, err := client.Connect()
		if err != nil {
			fmt.Printf("Failed to connect to crawler: %v\n", err)
			os.Exit(1)
		}
		defer conn.Close()

		switch *command {
		case "add":
			if *url == "" {
				fmt.Println("URL required for add command")
				os.Exit(1)
			}
			if err := client.AddURL(conn, *url); err != nil {
				fmt.Printf("Error adding URL: %v\n", err)
			} else {
				fmt.Printf("Added URL: %s\n", *url)
			}
		case "bulk":
			if *urls == "" {
				fmt.Println("URLs required for bulk command")
				os.Exit(1)
			}
			urlList := strings.Split(*urls, ",")
			for i, u := range urlList {
				urlList[i] = strings.TrimSpace(u)
			}
			if err := client.AddBulkURLs(conn, urlList); err != nil {
				fmt.Printf("Error adding URLs: %v\n", err)
			} else {
				fmt.Printf("Added %d URLs\n", len(urlList))
			}
		case "stats":
			stats, err := client.GetStats(conn)
			if err != nil {
				fmt.Printf("Error getting stats: %v\n", err)
			} else {
				data, _ := json.MarshalIndent(stats, "", "  ")
				fmt.Println(string(data))
			}
		default:
			fmt.Printf("Unknown command: %s\n", *command)
			os.Exit(1)
		}
	case "index":
		indexer := CreateIndexer()
		indexer.BuildIndex()
	case "server":
		server := CreateServer(*port)
		server.Start()
	case "proxy":
		proxy := CreateProxy(*port, "/tmp/exsearch.sock")
		proxy.Start()
	default:
		fmt.Println("Invalid mode. Use: server, proxy, crawl, index, distributed, or client")
		os.Exit(1)
	}
}
