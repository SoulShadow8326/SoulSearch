package main

import (
	"encoding/json"
	"net/http"
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
		Results   []SearchResult `json:"results"`
		Total     int            `json:"total"`
		TimeTaken string         `json:"time_taken"`
	}{
		Results:   results,
		Total:     len(results),
		TimeTaken: duration.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(struct {
		Suggestions []string `json:"suggestions"`
	}{
		Suggestions: suggestions,
	})
}
