package main

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)
import uarel "net/url"

var visited = make(map[string]bool)
var pages []PageData
var mu sync.Mutex
var wg sync.WaitGroup
var semaphore = make(chan struct{}, 64)
var hostAccess = make(map[string]time.Time)
var hostMu sync.Mutex

const politeDelay = 5 * time.Second

func Crawl(url string) {
	queue := []string{url}
	for len(queue) > 0 {
		var nextWave []string

		for _, u := range queue {
			wg.Add(1)
			semaphore <- struct{}{}

			go func(link string) {
				defer wg.Done()
				defer func() { <-semaphore }()

				robots := Respect(link)
				if robots != nil {
					group := robots.FindGroup("SoulSearchBot")
					if !group.Test(link) {
						return
					}
				}
				parsed, err := uarel.Parse(link)
				if err != nil {
					return
				}
				host := parsed.Host
				hostMu.Lock()
				last := hostAccess[host]
				wait := time.Until(last.Add(politeDelay))
				if wait > 0 {
					hostMu.Unlock()
					time.Sleep(wait)
					hostMu.Lock()
				}
				hostAccess[host] = time.Now()
				hostMu.Unlock()

				html, err := FetchHTML(link)
				if err != nil {
					return
				}

				mu.Lock()
				if visited[link] {
					mu.Unlock()
					return
				}
				visited[link] = true
				mu.Unlock()

				links := ExtractLinks(html, link)

				page := PageData{
					URL:      link,
					Title:    ExtractTitle(html, link),
					Content:  html,
					LinkList: links,
				}
				_ = StorePageData(page)
				_ = StoreLinks(page.URL, page.LinkList)

				mu.Lock()
				pages = append(pages, page)
				mu.Unlock()

				for _, l := range links {
					mu.Lock()
					if !visited[l] {
						nextWave = append(nextWave, l)
					}
					mu.Unlock()
				}

			}(u)
		}

		wg.Wait()
		queue = nextWave
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
