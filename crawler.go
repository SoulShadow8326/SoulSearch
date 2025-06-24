package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const saveEvery = 10

var shutdown = make(chan struct{})
var CrawlCount int

func FetchHTML(url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
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

func Crawl(url string) {
	defer wg.Done()
	select {
	case <-shutdown:
		return
	default:
	}
	fmt.Println("Entered Crawl() for:", url)

	mu.Lock()
	if visited[url] {
		mu.Unlock()
		return
	}
	visited[url] = true
	mu.Unlock()

	fmt.Println("Crawling:", url)

	html, err := FetchHTML(url)
	if err != nil {
		panic(fmt.Sprintf("FetchHTML failed for %s: %v", url, err))
	}
	fmt.Println("Fetched HTML content for:", url)

	links := ExtractLinks(html, url)
	fmt.Println("Number of links found:", len(links))

	mu.Lock()
	if !urlAlreadySaved(url) {
		pages = append(pages, PageData{URL: url, Links: links})
	}

	CrawlCount++
	shouldSave := CrawlCount%saveEvery == 0
	mu.Unlock()
	if shouldSave {
		fmt.Println("Checkpoint")
		err := saveToJSON("results.json", pages)
		if err != nil {
			fmt.Println("error saving", err)
		} else {
			fmt.Println("checkpointed")
		}
	}
	for _, link := range links {
		if strings.HasPrefix(link, "/") {
			continue
		}

		mu.Lock()
		if visited[link] {
			mu.Unlock()
			continue
		}
		visited[link] = true
		mu.Unlock()

		wg.Add(1)

		go func(l string) {
			select {
			case <-shutdown:
				wg.Done()
				return
			default:
				Crawl(l)
			}
		}(link)

	}
}

func setupCloseHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("signal interrupt")

		close(shutdown)

		wg.Wait()

		mu.Lock()
		defer mu.Unlock()

		err := saveToJSON("results.json", pages)
		if err != nil {
			fmt.Println("Error saving:", err)
		} else {
			fmt.Println("Saved to results.json")
		}
		os.Exit(0)
	}()
}
