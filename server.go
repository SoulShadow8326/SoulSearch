package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Pre-index some high-quality content for common queries
	server := &Server{
		port:   port,
		engine: engine,
	}

	go server.preIndexCommonQueries()

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

	log.Printf("Starting search for query: '%s'", query)
	startTime := time.Now()

	// First, check if we already have good results in the existing index
	existingResults, total, timeTaken := s.engine.SearchPaginated(query, 1, 10)

	// If we have authoritative results, return them immediately
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

	// Add timeout handling
	timeout := 25 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})
	var crawledDocs []CrawledDocument

	go func() {
		defer close(done)
		crawler := NewContentCrawler()
		seeds := crawler.GetQualitySeeds(query)
		log.Printf("Generated %d seeds for crawling", len(seeds))
		crawledDocs = crawler.CrawlContent(seeds)
		log.Printf("Crawled %d documents", len(crawledDocs))
	}()

	select {
	case <-done:
		// Crawling completed successfully
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

	// Remove script and style tags completely
	content = removeScriptAndStyleTags(content)

	// Remove Wikipedia-specific junk
	content = removeWikipediaJunk(content)

	// Remove HTML tags
	content = removeHtmlTags(content)

	// Clean up whitespace
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\t", " ")
	content = strings.ReplaceAll(content, "\r", " ")

	// Remove multiple spaces
	for strings.Contains(content, "  ") {
		content = strings.ReplaceAll(content, "  ", " ")
	}

	// Remove common junk patterns
	content = removeCommonJunk(content)

	words := strings.Fields(content)
	if len(words) > 500 {
		words = words[:500]
	}

	return strings.Join(words, " ")
}

func removeScriptAndStyleTags(html string) string {
	// Remove <script>...</script>
	for {
		start := strings.Index(html, "<script")
		if start == -1 {
			break
		}
		end := strings.Index(html[start:], "</script>")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+end+9:]
	}

	// Remove <style>...</style>
	for {
		start := strings.Index(html, "<style")
		if start == -1 {
			break
		}
		end := strings.Index(html[start:], "</style>")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+end+8:]
	}

	return html
}

func removeWikipediaJunk(content string) string {
	// Remove common Wikipedia patterns
	junkPatterns := []string{
		"hatnote{display:none!important}",
		"redirect here",
		"Doggy\" and \"Pooch\" redirect here",
		"\"Doggy\" and \"Pooch\" redirect here",
		"For other uses, see",
		"Not to be confused with",
		"This article is about",
		"Jump to navigation",
		"Jump to search",
		"From Wikipedia, the free encyclopedia",
		"Coordinates:",
		"mw-parser-output",
		"class=\"",
		"id=\"",
		"style=\"",
		"at DuckDuckGo",
		"Search results",
		"DuckDuckGo",
		"Also called the domestic",
		"selectively bred from",
		"during the Late Pleistocene",
		"by hunter-gatherers",
	}

	for _, pattern := range junkPatterns {
		content = strings.ReplaceAll(content, pattern, " ")
	}

	return content
}

func removeCommonJunk(content string) string {
	// Remove common web junk
	junkPatterns := []string{
		"Cookie",
		"Privacy Policy",
		"Terms of Service",
		"Sign up",
		"Log in",
		"Subscribe",
		"Newsletter",
		"Advertisement",
		"Sponsored",
		"Related Articles",
		"More Stories",
		"Comments",
		"Share this",
		"Follow us",
		"Like us",
		"Tweet",
		"Facebook",
		"Twitter",
		"LinkedIn",
		"Instagram",
	}

	contentLower := strings.ToLower(content)
	for _, pattern := range junkPatterns {
		patternLower := strings.ToLower(pattern)
		if strings.Contains(contentLower, patternLower) {
			// Remove the pattern case-insensitively
			content = removePatternCaseInsensitive(content, pattern)
		}
	}

	return content
}

func removePatternCaseInsensitive(text, pattern string) string {
	// Simple case-insensitive replacement
	lowerPattern := strings.ToLower(pattern)

	result := ""
	i := 0
	for i < len(text) {
		if i <= len(text)-len(pattern) && strings.ToLower(text[i:i+len(pattern)]) == lowerPattern {
			result += " "
			i += len(pattern)
		} else {
			result += string(text[i])
			i++
		}
	}

	return result
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

func (s *Server) preIndexCommonQueries() {
	log.Printf("Starting pre-indexing of common queries...")

	// Common topics that benefit from authoritative sources
	commonTopics := []struct {
		topic string
		urls  []string
	}{
		{
			topic: "duck",
			urls: []string{
				"https://en.wikipedia.org/wiki/Duck",
				"https://simple.wikipedia.org/wiki/Duck",
			},
		},
		{
			topic: "frog",
			urls: []string{
				"https://en.wikipedia.org/wiki/Frog",
				"https://simple.wikipedia.org/wiki/Frog",
			},
		},
		{
			topic: "cat",
			urls: []string{
				"https://en.wikipedia.org/wiki/Cat",
				"https://simple.wikipedia.org/wiki/Cat",
			},
		},
		{
			topic: "dog",
			urls: []string{
				"https://en.wikipedia.org/wiki/Dog",
				"https://simple.wikipedia.org/wiki/Dog",
			},
		},
		{
			topic: "hackclub",
			urls: []string{
				"https://hackclub.com",
				"https://en.wikipedia.org/wiki/Hack_Club",
			},
		},
		{
			topic: "python",
			urls: []string{
				"https://en.wikipedia.org/wiki/Python_(programming_language)",
				"https://docs.python.org/3/",
			},
		},
	}

	crawler := NewContentCrawler()
	var allDocs []CrawledDocument

	for _, topic := range commonTopics {
		log.Printf("Pre-indexing topic: %s", topic.topic)

		var seeds []SeedURL
		for _, url := range topic.urls {
			seeds = append(seeds, SeedURL{
				URL:         url,
				Priority:    0.9,
				Topic:       topic.topic,
				Source:      "preindex",
				Reliability: 0.95,
			})
		}

		docs := crawler.CrawlContent(seeds)
		allDocs = append(allDocs, docs...)
		log.Printf("Pre-indexed %d documents for topic: %s", len(docs), topic.topic)
	}

	if len(allDocs) > 0 {
		var pages []Page
		for _, doc := range allDocs {
			page := Page{
				URL:     doc.URL,
				Title:   doc.Title,
				Content: doc.Content,
				Crawled: doc.CrawledAt,
			}
			pages = append(pages, page)
		}

		index := s.indexPages(pages)
		s.engine.LoadIndex(index)
		log.Printf("Pre-indexing completed. Loaded %d documents into search index", len(pages))
	}
}

func (s *Server) hasAuthoritativeResults(results []SearchResult) bool {
	if len(results) == 0 {
		return false
	}

	// Check if the top result is from an authoritative source
	authoritativeDomains := []string{
		"wikipedia.org",
		"hackclub.com",
		"hackclub.org",
		"docs.python.org",
		"github.com",
		"stackoverflow.com",
		"britannica.com",
		"nationalgeographic.com",
		"smithsonianmag.com",
	}

	topResult := results[0]
	for _, domain := range authoritativeDomains {
		if strings.Contains(strings.ToLower(topResult.URL), domain) {
			// Also check if the score is reasonably high
			if topResult.Score > 15.0 {
				return true
			}
		}
	}

	return false
}
