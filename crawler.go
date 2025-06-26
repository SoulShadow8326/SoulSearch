package main

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"log"
)

var visited = make(map[string]bool)
var wg sync.WaitGroup

func Crawl() {
	for {
		rows, err := DB.Query(`SELECT url FROM pages WHERE crawled = 0 LIMIT 64`)
		if err != nil {
			log.Println("Failed to fetch uncrawled pages:", err)
			return
		}

		var urls []string
		for rows.Next() {
			var url string
			if err := rows.Scan(&url); err == nil {
				urls = append(urls, url)
			}
		}
		rows.Close()

		if len(urls) == 0 {
			log.Println("No more URLs to crawl.")
			return
		}

		var wg sync.WaitGroup
		sem := make(chan struct{}, 64)

		for _, url := range urls {
			wg.Add(1)
			sem <- struct{}{}

			go func(link string) {
				defer wg.Done()
				defer func() { <-sem }()

				if !AllowedByRobots(link) {
					return
				}

				html, err := FetchHTML(link)
				if err != nil {
					return
				}

				links := ExtractLinks(html, link)

				page := PageData{
					URL:     link,
					Title:   ExtractTitle(html, link),
					Content: html,
				}

				_ = StorePageData(page)
				QueueLinks(links)

				log.Println("crawled:", link)
			}(url)
		}

		wg.Wait()
	}
}



func FetchHTML(url string) (string, error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		url = "https://" + url
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "SoulSearchBot/1.0 (+https://example.com/bot)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
