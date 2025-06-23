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

func main() {
	setupCloseHandler()

	loadedPages, err := loadFromJSON("results.json")
	if err == nil {
		for _, page := range loadedPages {
			visited[page.URL] = true
			pages = append(pages, page)
		}
	}

	startURL := "en.wikipedia.org"

	fmt.Println("Starting crawl at:", startURL)

	wg.Add(1)
	go Crawl(startURL)
	wg.Wait()

	saveToJSON("results.json", pages)
}
