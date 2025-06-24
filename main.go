package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type PageData struct {
	URL   string   `json:"url"`
	Links []string `json:"links"`
}

var (
	pages   []PageData
	mu      sync.Mutex
	visited = make(map[string]bool)
	wg      sync.WaitGroup
)

func saveToJSON(filename string, data []PageData) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func loadFromJSON(filename string) ([]PageData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var data []PageData
	err = json.NewDecoder(file).Decode(&data)
	return data, err
}

func urlAlreadySaved(url string) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, page := range pages {
		if page.URL == url {
			return true
		}
	}
	return false
}

func main() {
	setupCloseHandler()

	loadedPages, err := loadFromJSON("results.json")
	if err == nil {
		for _, page := range loadedPages {
			if !urlAlreadySaved(page.URL) {
				pages = append(pages, page)
			}
		}
	}

	startURL := "en.wikipedia.org"
	fmt.Println("Starting crawl at:", startURL)

	wg.Add(1)
	go Crawl(startURL)
	wg.Wait()
	fmt.Println("Done crawling. Exiting.")
	err = saveToJSON("results.json", pages)
	if err != nil {
		fmt.Println("Error saving JSON:", err)
	} else {
		fmt.Println("Saved crawl results to results.json")
	}
}
