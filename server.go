package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Server struct {
	port   int
	engine *SearchEngine
}

type SearchRequest struct {
	Query string `json:"query"`
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
}

type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	TotalPages int            `json:"total_pages"`
	TimeTaken  string         `json:"time_taken"`
}

func NewServer(port int) *Server {
	if port == 0 {
		port = 8080
	}

	engine := NewSearchEngine()
	log.Println("Loading search index...")
	index := LoadIndex()
	engine.LoadIndex(index)
	log.Printf("Index loaded with %d terms and %d documents", len(index.Terms), len(index.Docs))

	return &Server{
		port:   port,
		engine: engine,
	}
}

func (s *Server) spaHandler(fs http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/favicon.ico" || r.URL.Path == "/manifest.json" || r.URL.Path == "/logo192.png" || r.URL.Path == "/logo512.png" || r.URL.Path == "/robots.txt" || r.URL.Path == "/asset-manifest.json" {
			fs.ServeHTTP(w, r)
			return
		}
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			fs.ServeHTTP(w, r)
			return
		}
		// Serve index.html for all other routes (SPA)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "./frontend/build/index.html")
	}
}

func (s *Server) Start() {
	http.HandleFunc("/api/search", s.handleSearch)
	http.HandleFunc("/api/suggestions", s.handleSuggestions)
	http.HandleFunc("/api/health", s.handleHealth)
	http.HandleFunc("/api/crawl", s.handleCrawl)
	http.HandleFunc("/api/index", s.handleIndex)
	http.HandleFunc("/api/analytics", s.handleAnalytics)
	http.HandleFunc("/api/analytics/advanced", s.handleAdvancedAnalytics)
	http.HandleFunc("/api/analytics/trends", s.handleQueryTrends)

	fs := http.FileServer(http.Dir("./frontend/build"))
	http.Handle("/static/", fs)
	http.HandleFunc("/", s.spaHandler(fs))

	fmt.Printf("Starting ExSearch server on http://localhost:%d...\n", s.port)
	log.Printf("Starting ExSearch HTTP server on port %d", s.port)

	indexer := NewIndexer()
	index := indexer.BuildIndex()
	if index != nil && len(index.Docs) > 0 {
		s.engine.LoadIndex(index)
		log.Printf("Loaded existing index with %d documents", len(index.Docs))
	} else {
		log.Printf("No existing index found, starting with empty search engine")
	}

	go func() {
		time.Sleep(2 * time.Second)
		// crawling disabled: CrawlFromSeed method removed

		sites := []string{
			"https://news.ycombinator.com",
			"https://en.wikipedia.org/wiki/Main_Page",
			"https://github.com/trending",
			"https://stackoverflow.com/questions",
			"https://www.reddit.com/r/programming",
		}

		for range sites {
			time.Sleep(1 * time.Second)
		}

		time.Sleep(10 * time.Second)
		newIndex := indexer.BuildIndex()
		if newIndex != nil && len(newIndex.Docs) > 0 {
			s.engine.LoadIndex(newIndex)
			log.Printf("Updated index with %d documents", len(newIndex.Docs))
		}
	}()

	fmt.Printf("ExSearch server running on http://localhost:%d\n", s.port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received search request: %s %s", r.Method, r.URL.Path)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode JSON body: %v", err)
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "Query parameter required", http.StatusBadRequest)
			return
		}
		req.Query = query
		req.Page = 1
		req.Limit = 10

		if pageStr := r.URL.Query().Get("page"); pageStr != "" {
			if page, err := strconv.Atoi(pageStr); err == nil {
				req.Page = page
			}
		}

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil {
				req.Limit = limit
			}
		}
	}

	log.Printf("Search request parsed: query='%s', page=%d, limit=%d", req.Query, req.Page, req.Limit)

	if req.Query == "" {
		http.Error(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 10
	}

	results, total, timeTaken := s.engine.SearchPaginated(req.Query, req.Page, req.Limit)

	log.Printf("Search completed in %s, found %d results on page %d of total %d", timeTaken, len(results), req.Page, total)

	totalPages := (total + req.Limit - 1) / req.Limit
	if totalPages == 0 {
		totalPages = 1
	}

	response := SearchResponse{
		Results:    results,
		Total:      total,
		Page:       req.Page,
		TotalPages: totalPages,
		TimeTaken:  timeTaken,
	}

	log.Printf("Sending response with %d results", len(response.Results))

	for i, result := range response.Results {
		log.Printf("Result %d: URL=%s, Title=%s, Snippet=%s, Score=%.4f",
			i+1, result.URL, result.Title, result.Snippet, result.Score)
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleSuggestions(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received suggestions request: %s %s", r.Method, r.URL.Path)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode JSON body: %v", err)
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "Query parameter required", http.StatusBadRequest)
			return
		}
		req.Query = query
		req.Page = 1
		req.Limit = 10

		if pageStr := r.URL.Query().Get("page"); pageStr != "" {
			if page, err := strconv.Atoi(pageStr); err == nil {
				req.Page = page
			}
		}

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil {
				req.Limit = limit
			}
		}
	}

	log.Printf("Suggestions request parsed: query='%s', page=%d, limit=%d", req.Query, req.Page, req.Limit)

	if req.Query == "" {
		http.Error(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 10
	}

	suggestions := s.engine.GetSuggestions(req.Query)

	log.Printf("Suggestions completed, found %d suggestions", len(suggestions))

	results := make([]SearchResult, 0, len(suggestions))
	for i, suggestion := range suggestions {
		results = append(results, SearchResult{
			URL:     "",
			Title:   suggestion,
			Snippet: "",
			Score:   0,
			Rank:    i + 1,
		})
	}

	response := SearchResponse{
		Results:    results,
		Total:      len(results),
		Page:       req.Page,
		TotalPages: 1,
		TimeTaken:  "0ms",
	}

	log.Printf("Sending response with %d suggestions", len(response.Results))

	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	response := map[string]string{
		"status":  "healthy",
		"service": "exsearch",
	}
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleCrawl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL      string `json:"url"`
		MaxPages int    `json:"max_pages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		req.URL = "https://example.com"
	}
	if req.MaxPages == 0 {
		req.MaxPages = 100
	}

	response := map[string]interface{}{
		"status":    "started",
		"url":       req.URL,
		"max_pages": req.MaxPages,
	}
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	indexer := NewIndexer()
	go indexer.BuildIndex()

	response := map[string]string{
		"status": "indexing started",
	}
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	analytics := s.engine.GetAnalytics()
	json.NewEncoder(w).Encode(analytics)
}

func (s *Server) handleAdvancedAnalytics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	analytics := s.engine.GetAdvancedAnalytics()
	json.NewEncoder(w).Encode(analytics)
}

func (s *Server) handleQueryTrends(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	trends := s.engine.GetQueryTrends()
	json.NewEncoder(w).Encode(trends)
}
