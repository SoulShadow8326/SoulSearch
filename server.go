package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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

func NewServer(port int) *Server {
	if port == 0 {
		port = 8080
	}

	engine := NewSearchEngine()

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

	log.Printf("Starting real-time crawling and indexing for query: '%s'", query)
	startTime := time.Now()

	searchURLs := generateSearchURLs(query)
	pages := s.crawlPages(searchURLs)

	log.Printf("Crawled %d pages", len(pages))

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

	index := s.indexPages(pages)
	s.engine.LoadIndex(index)

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

func (s *Server) crawlPages(urls []string) []Page {
	var pages []Page
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for _, rawURL := range urls {
		log.Printf("Crawling URL: %s", rawURL)

		resp, err := client.Get(rawURL)
		if err != nil {
			log.Printf("Error crawling %s: %v", rawURL, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Printf("Non-200 status for %s: %d", rawURL, resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading body for %s: %v", rawURL, err)
			continue
		}

		content := string(body)
		title := extractTitle(content)
		cleanContent := extractContent(content)

		if len(cleanContent) > 50 {
			page := Page{
				URL:     rawURL,
				Title:   title,
				Content: cleanContent,
				Crawled: time.Now(),
			}
			pages = append(pages, page)
			log.Printf("Successfully crawled: %s (title: %s, content length: %d)", rawURL, title, len(cleanContent))
		}

		time.Sleep(500 * time.Millisecond)
	}

	return pages
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

func extractTitle(html string) string {
	start := strings.Index(strings.ToLower(html), "<title>")
	if start == -1 {
		return "Untitled"
	}
	start += 7

	end := strings.Index(strings.ToLower(html[start:]), "</title>")
	if end == -1 {
		return "Untitled"
	}

	title := html[start : start+end]
	title = strings.TrimSpace(title)
	if title == "" {
		return "Untitled"
	}

	return title
}

func extractContent(html string) string {
	content := html

	content = removeHtmlTags(content)
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\t", " ")

	words := strings.Fields(content)
	if len(words) > 500 {
		words = words[:500]
	}

	return strings.Join(words, " ")
}

func removeHtmlTags(html string) string {
	var result strings.Builder
	inTag := false

	for _, char := range html {
		if char == '<' {
			inTag = true
			continue
		}
		if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}

	return result.String()
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

func generateSearchURLs(query string) []string {
	queryEncoded := strings.ReplaceAll(query, " ", "%20")

	urls := []string{
		"https://en.wikipedia.org/wiki/Special:Search?search=" + queryEncoded,
	}

	if len(strings.Fields(query)) == 1 {
		directWiki := "https://en.wikipedia.org/wiki/" + strings.Title(query)
		urls = append(urls, directWiki)
	}

	techTerms := map[string]bool{
		"programming": true, "code": true, "javascript": true, "python": true, "go": true,
		"react": true, "node": true, "api": true, "github": true, "git": true, "docker": true,
		"kubernetes": true, "database": true, "sql": true, "web": true, "framework": true,
		"library": true, "algorithm": true, "data": true, "structure": true, "software": true,
		"development": true, "frontend": true, "backend": true, "fullstack": true,
	}

	queryLower := strings.ToLower(query)
	for term := range techTerms {
		if strings.Contains(queryLower, term) {
			urls = append(urls, "https://stackoverflow.com/search?q="+queryEncoded)
			urls = append(urls, "https://github.com/search?q="+queryEncoded+"&type=repositories")
			break
		}
	}

	return urls
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
