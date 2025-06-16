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
	return &Server{
		port:   port,
		engine: NewSearchEngine(),
	}
}

func (s *Server) Start() {
	http.HandleFunc("/api/search", s.handleSearch)
	http.HandleFunc("/api/health", s.handleHealth)

	fmt.Printf("SoulSearch API running on http://localhost:%d\n", s.port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(s.port), nil))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
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

	start := time.Now()
	results := s.engine.Search(req.Query, req.Limit)
	timeTaken := time.Since(start).String()

	totalPages := (len(results) + req.Limit - 1) / req.Limit
	if totalPages == 0 {
		totalPages = 1
	}

	response := SearchResponse{
		Results:    results,
		Total:      len(results),
		Page:       req.Page,
		TotalPages: totalPages,
		TimeTaken:  timeTaken,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	response := map[string]string{
		"status":  "healthy",
		"service": "soulsearch",
	}
	json.NewEncoder(w).Encode(response)
}
