package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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

func CreateServer(port int) *Server {
	if port == 0 {
		port = 8080
	}

	engine := CreateSearchEngine()

	server := &Server{
		port:   port,
		engine: engine,
	}

	server.loadPersistedIndex()

	return server
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
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "./frontend/build/index.html")
	}
}

func (s *Server) Start() {
	http.HandleFunc("/api/dynamic-search", s.handleDynamicSearch)
	http.HandleFunc("/api/suggestions", s.handleSuggestions)

	fs := http.FileServer(http.Dir("./frontend/build"))
	http.Handle("/static/", fs)
	http.HandleFunc("/", s.spaHandler(fs))

	fmt.Printf("Starting SoulSearch server on http://localhost:%d...\n", s.port)
	log.Printf("Starting SoulSearch HTTP server on port %d", s.port)

	fmt.Printf("SoulSearch server running on http://localhost:%d\n", s.port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil))
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

	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var query string
	if r.Method == "GET" {
		query = r.URL.Query().Get("q")
	} else {
		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("Failed to decode JSON body: %v", err)
			query = r.URL.Query().Get("q")
		} else {
			query = req.Query
		}
	}

	if query == "" {
		http.Error(w, "Query cannot be empty", http.StatusBadRequest)
		return
	}

	suggestions := s.engine.GetSuggestions(query)

	log.Printf("Suggestions completed, found %d suggestions", len(suggestions))

	response := map[string]interface{}{
		"suggestions": suggestions,
	}

	log.Printf("Sending response with %d suggestions", len(suggestions))

	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleDynamicSearch(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received dynamic search request: %s %s", r.Method, r.URL.Path)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter required", http.StatusBadRequest)
		return
	}

	log.Printf("Starting search for query: '%s'", query)
	startTime := time.Now()

	existingResults, total, timeTaken := s.engine.SearchPaginated(query, 1, 10)

	if s.hasAuthoritativeResults(existingResults) {
		log.Printf("Found authoritative results in existing index, returning immediately")

		for i := range existingResults {
			if existingResults[i].Snippet == "" {
				existingResults[i].Snippet = s.generateSimpleSnippet(existingResults[i].URL, query)
			}
		}

		response := SearchResponse{
			Results:    existingResults,
			Total:      total,
			Page:       1,
			TotalPages: (total + 9) / 10,
			TimeTaken:  timeTaken,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("No authoritative results found, starting real-time crawling for query: '%s'", query)

	timeout := 25 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})
	var crawledDocs []CrawledDocument

	go func() {
		defer close(done)
		crawler := CreateContentCrawler()
		seeds := crawler.GetQualitySeeds(query)
		log.Printf("Generated %d seeds for crawling", len(seeds))
		crawledDocs = crawler.CrawlContent(seeds)
		log.Printf("Crawled %d documents", len(crawledDocs))
	}()

	select {
	case <-done:
	case <-ctx.Done():
		log.Printf("Crawling timed out after %v", timeout)
		response := SearchResponse{
			Results:    []SearchResult{},
			Total:      0,
			Page:       1,
			TotalPages: 0,
			TimeTaken:  time.Since(startTime).String(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("Crawled %d documents", len(crawledDocs))

	var pages []Page
	for _, doc := range crawledDocs {
		page := Page{
			URL:     doc.URL,
			Title:   doc.Title,
			Content: doc.Content,
			Crawled: doc.CrawledAt,
		}
		pages = append(pages, page)
	}

	if len(pages) == 0 {
		log.Printf("No pages crawled, returning empty results")
		response := SearchResponse{
			Results:    []SearchResult{},
			Total:      0,
			Page:       1,
			TotalPages: 0,
			TimeTaken:  time.Since(startTime).String(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	s.persistCrawledContent(pages)

	results, total, timeTaken := s.engine.SearchPaginated(query, 1, 10)

	for i := range results {
		if results[i].Snippet == "" {
			results[i].Snippet = s.generateSimpleSnippet(results[i].URL, query)
		}
	}

	log.Printf("Dynamic search completed in %s, found %d results", time.Since(startTime).String(), len(results))

	response := SearchResponse{
		Results:    results,
		Total:      total,
		Page:       1,
		TotalPages: (total + 9) / 10,
		TimeTaken:  timeTaken,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) indexPages(pages []Page) *InvertedIndex {
	index := &InvertedIndex{
		Terms: make(map[string][]TermFreq),
		Docs:  make(map[string]Document),
	}

	stopWords := loadStopWords()

	for _, page := range pages {
		doc := Document{
			URL:      page.URL,
			Title:    page.Title,
			Content:  page.Content,
			Length:   len(strings.Fields(page.Content)),
			PageRank: 1.0,
		}

		index.Docs[page.URL] = doc

		allText := page.Title + " " + page.Content
		words := tokenize(allText)

		termFreqs := make(map[string]int)
		for _, word := range words {
			word = strings.ToLower(word)
			if len(word) > 2 && !stopWords[word] {
				termFreqs[word]++
			}
		}

		for term, freq := range termFreqs {
			score := float64(freq) / float64(len(words))
			if strings.Contains(strings.ToLower(page.Title), term) {
				score *= 2.0
			}

			index.Terms[term] = append(index.Terms[term], TermFreq{
				URL:   page.URL,
				Score: score,
			})
		}
	}

	return index
}

func tokenize(text string) []string {
	var words []string
	var current strings.Builder

	for _, char := range text {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			current.WriteRune(char)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

func loadStopWords() map[string]bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"this": true, "that": true, "these": true, "those": true, "it": true, "they": true,
		"he": true, "she": true, "we": true, "you": true, "i": true, "me": true, "my": true,
		"from": true, "as": true, "all": true, "any": true, "can": true, "may": true,
	}
	return stopWords
}

func (s *Server) generateSimpleSnippet(url, query string) string {
	s.engine.mu.RLock()
	defer s.engine.mu.RUnlock()

	doc, exists := s.engine.index.Docs[url]
	if !exists {
		return "No content available"
	}

	content := doc.Content
	if content == "" {
		return "No content available"
	}

	queryTerms := strings.Fields(strings.ToLower(query))
	sentences := strings.Split(content, ".")

	bestSentence := ""
	maxMatches := 0

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < 20 || len(sentence) > 300 {
			continue
		}

		sentenceLower := strings.ToLower(sentence)
		matches := 0

		for _, term := range queryTerms {
			if strings.Contains(sentenceLower, term) {
				matches++
			}
		}

		if matches > maxMatches {
			maxMatches = matches
			bestSentence = sentence
		}
	}

	if bestSentence == "" && len(sentences) > 0 {
		for _, sentence := range sentences {
			sentence = strings.TrimSpace(sentence)
			if len(sentence) >= 50 && len(sentence) <= 200 {
				bestSentence = sentence
				break
			}
		}
	}

	if bestSentence == "" {
		words := strings.Fields(content)
		if len(words) > 20 {
			bestSentence = strings.Join(words[:20], " ") + "..."
		} else {
			bestSentence = content
		}
	}

	if len(bestSentence) > 200 {
		words := strings.Fields(bestSentence)
		if len(words) > 25 {
			bestSentence = strings.Join(words[:25], " ") + "..."
		}
	}

	return bestSentence
}

func (s *Server) hasAuthoritativeResults(results []SearchResult) bool {
	if len(results) == 0 {
		return false
	}

	topResult := results[0]
	if topResult.Score < 20.0 {
		return false
	}

	authoritativeDomains := []string{
		"wikipedia.org",
		"hackclub.com",
		"hackclub.org",
		"docs.python.org",
		"cdc.gov",
		"nih.gov",
		"github.com",
		"stackoverflow.com",
		"britannica.com",
	}

	authoritativeCount := 0
	for _, result := range results {
		for _, domain := range authoritativeDomains {
			if strings.Contains(strings.ToLower(result.URL), domain) && result.Score > 15.0 {
				authoritativeCount++
				break
			}
		}
		if authoritativeCount >= 1 {
			break
		}
	}

	return authoritativeCount >= 1
}

func (s *Server) loadPersistedIndex() {
	if index := s.loadIndexFromDisk(); index != nil {
		s.engine.LoadIndex(index)
		log.Printf("Loaded persisted index with %d documents", len(index.Docs))
	} else {
		log.Printf("No persisted index found, starting with empty index")
	}
}

func (s *Server) persistCrawledContent(pages []Page) {
	currentIndex := s.engine.GetCurrentIndex()
	if currentIndex == nil {
		currentIndex = &InvertedIndex{
			Terms: make(map[string][]TermFreq),
			Docs:  make(map[string]Document),
		}
	}

	newIndex := s.indexPages(pages)
	s.mergeIndexes(currentIndex, newIndex)

	s.saveIndexToDisk(currentIndex)
	s.engine.LoadIndex(currentIndex)
}

func (s *Server) mergeIndexes(existing, new *InvertedIndex) {
	for url, doc := range new.Docs {
		existing.Docs[url] = doc
	}

	for term, termFreqs := range new.Terms {
		if existingFreqs, exists := existing.Terms[term]; exists {
			urlMap := make(map[string]bool)
			for _, tf := range existingFreqs {
				urlMap[tf.URL] = true
			}

			for _, tf := range termFreqs {
				if !urlMap[tf.URL] {
					existing.Terms[term] = append(existing.Terms[term], tf)
				}
			}
		} else {
			existing.Terms[term] = termFreqs
		}
	}
}

func (s *Server) saveIndexToDisk(index *InvertedIndex) error {
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}

	docsFile, err := os.Create("data/documents.json")
	if err != nil {
		return err
	}
	defer docsFile.Close()

	if err := json.NewEncoder(docsFile).Encode(index.Docs); err != nil {
		return err
	}

	termsFile, err := os.Create("data/terms.json")
	if err != nil {
		return err
	}
	defer termsFile.Close()

	if err := json.NewEncoder(termsFile).Encode(index.Terms); err != nil {
		return err
	}

	log.Printf("Persisted index with %d documents and %d terms", len(index.Docs), len(index.Terms))
	return nil
}

func (s *Server) loadIndexFromDisk() *InvertedIndex {
	docsFile, err := os.Open("data/documents.json")
	if err != nil {
		return nil
	}
	defer docsFile.Close()

	var docs map[string]Document
	if err := json.NewDecoder(docsFile).Decode(&docs); err != nil {
		return nil
	}

	termsFile, err := os.Open("data/terms.json")
	if err != nil {
		return nil
	}
	defer termsFile.Close()

	var terms map[string][]TermFreq
	if err := json.NewDecoder(termsFile).Decode(&terms); err != nil {
		return nil
	}

	return &InvertedIndex{
		Terms: terms,
		Docs:  docs,
	}
}
