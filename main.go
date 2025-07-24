package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

var crawlEnabled = false

func main() {
	InitDB()
	LoadVisitedFromDB()

	if crawlEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			QueueLinks([]string{"https://en.wikipedia.org/wiki/Main_Page"})
			Crawl()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			time.Sleep(5 * time.Minute)
			ComputePageRank(20, 0.85)
		}
	}()

	http.HandleFunc("/search", handleSearch)
	http.HandleFunc("/crawl", handleCrawl)
	http.HandleFunc("/api/dynamic-search", handleSearch)
	http.HandleFunc("/api/suggestions", handleSuggestions)

	fs := http.FileServer(http.Dir("./frontend/build"))
	http.Handle("/static/", fs)
	http.Handle("/asset-manifest.json", fs)
	http.Handle("/favicon.ico", fs)
	http.Handle("/manifest.json", fs)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/search") || strings.HasPrefix(r.URL.Path, "/crawl") {
			http.NotFound(w, r)
			return
		}
		indexPath := "./frontend/build/index.html"
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
		http.NotFound(w, r)
	})

	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			panic(err)
		}
	}()

	wg.Wait()
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing query", http.StatusBadRequest)
		return
	}

	start := time.Now()
	results := Search(query)
	duration := time.Since(start)

	response := struct {
		Results []SearchResult `json:"results"`
		Total int `json:"total"`
		TimeTaken string `json:"time_taken"`
	}{
		Results:   results,
		Total:     len(results),
		TimeTaken: duration.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleCrawl(w http.ResponseWriter, r *http.Request) {
	if !crawlEnabled {
		http.Error(w, "Crawling disabled", http.StatusForbidden)
		return
	}

	url := r.URL.Query().Get("url")
	if url != "" {
		go func() {
			QueueLinks([]string{url})
			Crawl()
		}()
		w.Write([]byte("Crawling started"))
		return
	}
	http.Error(w, "Missing ?url=", http.StatusBadRequest)
}

func handleSuggestions(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	if query == "" || len(query) < 2 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Suggestions []string `json:"suggestions"`
		}{Suggestions: []string{}})
		return
	}

	rows, err := DB.Query(`SELECT DISTINCT term FROM inverted_index WHERE term LIKE ? LIMIT 10`, query+"%")
	if err != nil {
		http.Error(w, "Failed to fetch suggestions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var suggestions []string
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err == nil {
			suggestions = append(suggestions, term)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Suggestions []string `json:"suggestions"`
	}{
		Suggestions: suggestions,
	})
}
