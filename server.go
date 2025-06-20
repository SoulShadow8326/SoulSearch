package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Server struct {
	port        int
	engine      *SearchEngine
	crawlerSock string
	crawlerConn net.Conn
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
		port:        port,
		engine:      engine,
		crawlerSock: "/tmp/soulsearch.sock",
	}

	server.loadPersistedIndex()
	server.connectToCrawler()
	server.startIndexRefresher()

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

	http.ListenAndServe(":8080", nil)
}

func (s *Server) handleSuggestions(w http.ResponseWriter, r *http.Request) {
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

	response := map[string]interface{}{
		"suggestions": suggestions,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleDynamicSearch(w http.ResponseWriter, r *http.Request) {
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

	startTime := time.Now()

	existingResults, total, timeTaken := s.engine.SearchPaginated(query, 1, 10)

	if total > 0 && len(existingResults) > 0 {
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

	go s.triggerCrawlForQuery(query)

	// Try to get results from crawler's live index immediately
	liveResults := s.searchViaCrawler(query)
	if len(liveResults) > 0 {
		response := SearchResponse{
			Results:    liveResults,
			Total:      len(liveResults),
			Page:       1,
			TotalPages: 1,
			TimeTaken:  time.Since(startTime).String(),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	time.Sleep(2 * time.Second)

	existingResults, total, timeTaken = s.engine.SearchPaginated(query, 1, 10)

	if total > 0 && len(existingResults) > 0 {
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

	timeout := 8 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check local index first
			existingResults, total, timeTaken = s.engine.SearchPaginated(query, 1, 10)
			if total > 0 && len(existingResults) > 0 {
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

			// Check live crawler index
			liveResults := s.searchViaCrawler(query)
			if len(liveResults) > 0 {
				response := SearchResponse{
					Results:    liveResults,
					Total:      len(liveResults),
					Page:       1,
					TotalPages: 1,
					TimeTaken:  time.Since(startTime).String(),
				}
				json.NewEncoder(w).Encode(response)
				return
			}
		case <-ctx.Done():
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
	}

	response := SearchResponse{
		Results:    existingResults,
		Total:      total,
		Page:       1,
		TotalPages: (total + 9) / 10,
		TimeTaken:  time.Since(startTime).String(),
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) indexPages(pages []Page) *InvertedIndex {
	index := &InvertedIndex{
		Terms: &sync.Map{},
		Docs:  &sync.Map{},
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

		index.Docs.Store(page.URL, doc)

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

			termFreq := TermFreq{
				URL:   page.URL,
				Score: score,
			}

			if existing, exists := index.Terms.Load(term); exists {
				termFreqs := existing.([]TermFreq)
				termFreqs = append(termFreqs, termFreq)
				index.Terms.Store(term, termFreqs)
			} else {
				index.Terms.Store(term, []TermFreq{termFreq})
			}
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

	docInterface, exists := s.engine.index.Docs.Load(url)
	if !exists {
		return "No content available"
	}

	doc := docInterface.(Document)
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
	}
}

func (s *Server) persistCrawledContent(pages []Page) {
	currentIndex := s.engine.GetCurrentIndex()
	if currentIndex == nil {
		currentIndex = &InvertedIndex{
			Terms: &sync.Map{},
			Docs:  &sync.Map{},
		}
	}

	newIndex := s.indexPages(pages)
	s.mergeIndexes(currentIndex, newIndex)

	s.saveIndexToDisk(currentIndex)
	s.engine.LoadIndex(currentIndex)
}

func (s *Server) mergeIndexes(existing, new *InvertedIndex) {
	new.Docs.Range(func(key, value interface{}) bool {
		existing.Docs.Store(key, value)
		return true
	})

	new.Terms.Range(func(key, value interface{}) bool {
		term := key.(string)
		termFreqs := value.([]TermFreq)

		if existingValue, exists := existing.Terms.Load(term); exists {
			existingFreqs := existingValue.([]TermFreq)
			urlMap := make(map[string]struct{})
			for _, tf := range existingFreqs {
				urlMap[tf.URL] = struct{}{}
			}

			for _, tf := range termFreqs {
				if _, exists := urlMap[tf.URL]; !exists {
					existingFreqs = append(existingFreqs, tf)
				}
			}
			existing.Terms.Store(term, existingFreqs)
		} else {
			existing.Terms.Store(term, termFreqs)
		}
		return true
	})
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

	docs := make(map[string]Document)
	index.Docs.Range(func(key, value interface{}) bool {
		docs[key.(string)] = value.(Document)
		return true
	})

	if err := json.NewEncoder(docsFile).Encode(docs); err != nil {
		return err
	}

	termsFile, err := os.Create("data/terms.json")
	if err != nil {
		return err
	}
	defer termsFile.Close()

	terms := make(map[string][]TermFreq)
	index.Terms.Range(func(key, value interface{}) bool {
		terms[key.(string)] = value.([]TermFreq)
		return true
	})

	if err := json.NewEncoder(termsFile).Encode(terms); err != nil {
		return err
	}

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

	index := &InvertedIndex{
		Terms: &sync.Map{},
		Docs:  &sync.Map{},
	}

	for url, doc := range docs {
		index.Docs.Store(url, doc)
	}

	for term, termFreqs := range terms {
		index.Terms.Store(term, termFreqs)
	}

	return index
}

func (s *Server) connectToCrawler() {
	conn, err := net.Dial("unix", s.crawlerSock)
	if err != nil {
		return
	}
	s.crawlerConn = conn
}

func (s *Server) triggerCrawlForQuery(query string) {
	if s.crawlerConn == nil {
		s.connectToCrawler()
		if s.crawlerConn == nil {
			return
		}
	}

	urls := s.generateSearchURLs(query)
	if len(urls) == 0 {
		return
	}

	msg := map[string]interface{}{
		"type":    "BULK_ADD",
		"payload": urls,
	}

	data, _ := json.Marshal(msg)
	s.crawlerConn.Write(append(data, '\n'))
}

func (s *Server) generateSearchURLs(query string) []string {
	encodedQuery := strings.ReplaceAll(query, " ", "+")

	urls := []string{
		"https://en.wikipedia.org/wiki/Special:Search?search=" + encodedQuery,
		"https://stackoverflow.com/search?q=" + encodedQuery,
		"https://github.com/search?q=" + encodedQuery,
		"https://www.reddit.com/search/?q=" + encodedQuery,
	}

	directArticles := s.generateDirectArticleURLs(query)
	urls = append(urls, directArticles...)

	return urls
}

func (s *Server) generateDirectArticleURLs(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))

	directMappings := map[string][]string{
		"duck":           {"https://en.wikipedia.org/wiki/Duck"},
		"what is a duck": {"https://en.wikipedia.org/wiki/Duck", "https://en.wikipedia.org/wiki/Anatidae"},
		"cat":            {"https://en.wikipedia.org/wiki/Cat"},
		"dog":            {"https://en.wikipedia.org/wiki/Dog"},
		"bird":           {"https://en.wikipedia.org/wiki/Bird"},
		"animal":         {"https://en.wikipedia.org/wiki/Animal"},
		"programming":    {"https://en.wikipedia.org/wiki/Computer_programming"},
		"python":         {"https://en.wikipedia.org/wiki/Python_(programming_language)"},
		"javascript":     {"https://en.wikipedia.org/wiki/JavaScript"},
		"computer":       {"https://en.wikipedia.org/wiki/Computer"},
		"science":        {"https://en.wikipedia.org/wiki/Science"},
		"technology":     {"https://en.wikipedia.org/wiki/Technology"},
	}

	if urls, exists := directMappings[query]; exists {
		return urls
	}

	if strings.Contains(query, "duck") {
		return []string{"https://en.wikipedia.org/wiki/Duck", "https://en.wikipedia.org/wiki/Anatidae"}
	}
	if strings.Contains(query, "cat") {
		return []string{"https://en.wikipedia.org/wiki/Cat"}
	}
	if strings.Contains(query, "dog") {
		return []string{"https://en.wikipedia.org/wiki/Dog"}
	}
	if strings.Contains(query, "bird") {
		return []string{"https://en.wikipedia.org/wiki/Bird"}
	}

	return []string{}
}

func (s *Server) searchViaCrawler(query string) []SearchResult {
	if s.crawlerConn == nil {
		s.connectToCrawler()
		if s.crawlerConn == nil {
			return nil
		}
	}

	msg := map[string]interface{}{
		"type": "SEARCH",
		"payload": map[string]interface{}{
			"query": query,
			"limit": 10,
		},
	}

	data, _ := json.Marshal(msg)
	_, err := s.crawlerConn.Write(append(data, '\n'))
	if err != nil {
		s.crawlerConn = nil
		return nil
	}

	// Set a short timeout for the response
	s.crawlerConn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Read response
	buffer := make([]byte, 4096)
	n, err := s.crawlerConn.Read(buffer)
	if err != nil {
		return nil
	}

	// Reset deadline
	s.crawlerConn.SetReadDeadline(time.Time{})

	var response map[string]interface{}
	if err := json.Unmarshal(buffer[:n], &response); err != nil {
		return nil
	}

	// Check if response contains results
	if response["type"] == "SEARCH_RESULTS" {
		if payload, ok := response["payload"].(map[string]interface{}); ok {
			if results, ok := payload["results"].([]interface{}); ok {
				var searchResults []SearchResult
				for _, r := range results {
					if resultMap, ok := r.(map[string]interface{}); ok {
						result := SearchResult{
							URL:     getString(resultMap, "url"),
							Title:   getString(resultMap, "title"),
							Snippet: getString(resultMap, "snippet"),
							Score:   getFloat64(resultMap, "score"),
						}
						if result.URL != "" {
							searchResults = append(searchResults, result)
						}
					}
				}
				return searchResults
			}
		}
	}

	return nil
}

func (s *Server) startIndexRefresher() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.refreshIndexFromSharedSource()
			}
		}
	}()
}

func (s *Server) refreshIndexFromSharedSource() {
	sharedIndex := GetGlobalSharedIndex()
	if sharedIndex != nil {
		currentIndex := s.engine.GetCurrentIndex()
		if currentIndex == nil {
			currentIndex = &InvertedIndex{
				Terms: &sync.Map{},
				Docs:  &sync.Map{},
			}
		}

		newDocs := 0
		sharedIndex.GetIndex().Docs.Range(func(key, value interface{}) bool {
			url := key.(string)
			doc := value.(Document)

			if _, exists := currentIndex.Docs.Load(url); !exists {
				currentIndex.Docs.Store(url, doc)
				newDocs++

				tokens := tokenizeText(doc.Title + " " + doc.Content)
				for _, token := range tokens {
					if len(token) > 2 {
						var termFreqs []TermFreq
						if existing, ok := currentIndex.Terms.Load(token); ok {
							termFreqs = existing.([]TermFreq)
						}

						found := false
						for i := range termFreqs {
							if termFreqs[i].URL == doc.URL {
								termFreqs[i].Score += 1.0
								found = true
								break
							}
						}

						if !found {
							termFreqs = append(termFreqs, TermFreq{
								URL:   doc.URL,
								Score: 1.0,
							})
						}

						currentIndex.Terms.Store(token, termFreqs)
					}
				}
			}
			return true
		})

		if newDocs > 0 {
			s.engine.LoadIndex(currentIndex)
		}
	}
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	if val, ok := m[key].(int); ok {
		return float64(val)
	}
	return 0.0
}
