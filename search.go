package main

import (
	"bufio"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type SearchResult struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	Rank    int     `json:"rank"`
}

type SearchEngine struct {
	index        *InvertedIndex
	mu           sync.RWMutex
	stopWords    map[string]bool
	pageRank     map[string]float64
	linkGraph    map[string][]string
	synonyms     map[string][]string
	queryCache   map[string][]SearchResult
	cacheTimeout map[string]time.Time
	resultPool   sync.Pool
	idfScores    map[string]float64
	totalDocs    int
	analytics    map[string]int
	cacheHits    int
	totalQueries int
	termCounts   map[string]int
	docLengths   map[string]int
	avgDocLength float64
}

type WordNetResponse struct {
	Word     string   `json:"word"`
	Synonyms []string `json:"synonyms"`
}

type DataMuseResponse struct {
	Word  string   `json:"word"`
	Score float64  `json:"score"`
	Tags  []string `json:"tags"`
}

type ExternalDataLoader struct {
}

func NewSearchEngine() *SearchEngine {
	stopWords := loadStopWordsFromFile()
	synonyms := loadSynonymsFromFile()

	engine := &SearchEngine{
		stopWords:    stopWords,
		pageRank:     make(map[string]float64),
		linkGraph:    make(map[string][]string),
		synonyms:     synonyms,
		queryCache:   make(map[string][]SearchResult),
		cacheTimeout: make(map[string]time.Time),
		resultPool: sync.Pool{
			New: func() interface{} {
				return make([]SearchResult, 0, 100)
			},
		},
		idfScores:    make(map[string]float64),
		totalDocs:    0,
		analytics:    make(map[string]int),
		cacheHits:    0,
		totalQueries: 0,
		termCounts:   make(map[string]int),
		docLengths:   make(map[string]int),
		avgDocLength: 0,
	}

	log.Printf("Search engine initialized with %d stop words and %d synonym groups", len(stopWords), len(synonyms))

	go engine.backgroundMaintenance()

	return engine
}

func loadStopWordsFromFile() map[string]bool {
	stopWords := make(map[string]bool)

	file, err := os.Open("data/stopwords.txt")
	if err != nil {
		log.Printf("Error opening stopwords file: %v", err)
		return stopWords
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" {
			stopWords[word] = true
		}
	}

	log.Printf("Loaded %d stop words from file", len(stopWords))
	return stopWords
}

func loadSynonymsFromFile() map[string][]string {
	synonyms := make(map[string][]string)

	file, err := os.Open("data/synonyms.txt")
	if err != nil {
		log.Printf("Error opening synonyms file: %v", err)
		return synonyms
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				word := strings.TrimSpace(parts[0])
				synList := strings.Split(parts[1], ",")
				var cleanSyns []string
				for _, syn := range synList {
					syn = strings.TrimSpace(syn)
					if syn != "" {
						cleanSyns = append(cleanSyns, syn)
					}
				}
				if len(cleanSyns) > 0 {
					synonyms[word] = cleanSyns
				}
			}
		}
	}

	log.Printf("Loaded %d synonym groups from file", len(synonyms))
	return synonyms
}

func (se *SearchEngine) LoadIndex(index *InvertedIndex) {
	se.index = index
	se.totalDocs = len(index.Docs)
	se.computeIDF()

	log.Printf("Search engine loaded with %d terms, %d documents", len(se.index.Terms), se.totalDocs)

	termCount := 0
	for term := range se.index.Terms {
		if termCount < 10 {
			log.Printf("Sample indexed term: '%s'", term)
		}
		termCount++
		if termCount >= 10 {
			break
		}
	}
}

func (se *SearchEngine) computeIDF() {
	se.idfScores = make(map[string]float64)
	for term, termFreqs := range se.index.Terms {
		df := len(termFreqs)
		if df > 0 {
			se.idfScores[term] = math.Log(float64(se.totalDocs) / float64(df))
		}
	}
}

func (se *SearchEngine) Search(query string, limit int) ([]SearchResult, string) {
	start := time.Now()

	results := se.SearchAdvanced(query, limit)

	elapsed := time.Since(start)
	return results, elapsed.String()
}

// SearchPaginated performs a paginated search with proper offset calculation
func (se *SearchEngine) SearchPaginated(query string, page, limit int) ([]SearchResult, int, string) {
	start := time.Now()
	se.totalQueries++

	log.Printf("Starting paginated search for query: '%s', page: %d, limit: %d", query, page, limit)

	if se.index == nil || len(se.index.Terms) == 0 {
		log.Printf("Index is nil or empty")
		return nil, 0, time.Since(start).String()
	}

	// Check cache for all results first
	cacheKey := strings.ToLower(query)
	var allResults []SearchResult

	se.mu.RLock()
	if cached, exists := se.queryCache[cacheKey]; exists {
		if timeout, timeExists := se.cacheTimeout[cacheKey]; !timeExists || time.Since(timeout) < 5*time.Minute {
			se.mu.RUnlock()
			se.mu.Lock()
			se.cacheHits++
			se.mu.Unlock()
			log.Printf("Cache hit for paginated query: '%s' (hit rate: %.1f%%)", query, float64(se.cacheHits)/float64(se.totalQueries)*100)
			allResults = cached
		} else {
			se.mu.RUnlock()
			allResults = se.SearchAdvanced(query, 10000)
		}
	} else {
		se.mu.RUnlock()
		se.analytics[query]++
		allResults = se.SearchAdvanced(query, 10000)
	}

	total := len(allResults)

	// Calculate offset
	offset := (page - 1) * limit
	if offset >= total {
		return []SearchResult{}, total, time.Since(start).String()
	}

	// Calculate end index
	end := offset + limit
	if end > total {
		end = total
	}

	// Return paginated slice
	paginatedResults := allResults[offset:end]

	// Update ranks for the paginated results
	for i := range paginatedResults {
		paginatedResults[i].Rank = offset + i + 1
	}

	elapsed := time.Since(start)
	log.Printf("Paginated search completed in %v, returning results %d-%d of %d total", elapsed, offset+1, end, total)
	return paginatedResults, total, elapsed.String()
}

func (se *SearchEngine) SearchAdvanced(query string, maxResults int) []SearchResult {
	start := time.Now()
	se.totalQueries++

	log.Printf("Starting advanced search for query: '%s'", query)

	if se.index == nil || len(se.index.Terms) == 0 {
		log.Printf("Index is nil or empty")
		return nil
	}

	cacheKey := strings.ToLower(query)

	se.mu.RLock()
	if cached, exists := se.queryCache[cacheKey]; exists {
		if timeout, timeExists := se.cacheTimeout[cacheKey]; !timeExists || time.Since(timeout) < 5*time.Minute {
			se.mu.RUnlock()
			se.mu.Lock()
			se.cacheHits++
			se.mu.Unlock()
			log.Printf("Cache hit for query: '%s' (hit rate: %.1f%%)", query, float64(se.cacheHits)/float64(se.totalQueries)*100)
			return se.limitResults(cached, maxResults)
		}
	}
	se.mu.RUnlock()

	se.analytics[query]++

	queryTerms := se.processAdvancedQuery(query)
	log.Printf("Processed query terms: %v", queryTerms)

	candidates := se.findAdvancedCandidates(queryTerms)
	log.Printf("Found %d candidates", len(candidates))

	if len(candidates) == 0 && len(queryTerms) == 1 {
		log.Printf("No candidates for single term, trying broader search")
		singleTerm := queryTerms[0]
		for indexTerm := range se.index.Terms {
			if strings.Contains(indexTerm, singleTerm) || strings.Contains(singleTerm, indexTerm) {
				if docList := se.index.Terms[indexTerm]; len(docList) > 0 {
					for _, termFreq := range docList {
						candidates[termFreq.URL] = true
					}
					if len(candidates) >= 20 {
						break
					}
				}
			}
		}
		log.Printf("Broader search found %d candidates", len(candidates))
	}

	if len(candidates) == 0 {
		return nil
	}

	results := se.scoreAdvancedResults(queryTerms, candidates, query)
	log.Printf("Scored %d documents", len(results))

	se.optimizedSort(results)

	for i := range results {
		results[i].Rank = i + 1
	}

	se.mu.Lock()
	if len(se.queryCache) > 1000 {
		se.queryCache = make(map[string][]SearchResult)
		se.cacheTimeout = make(map[string]time.Time)
	}
	se.queryCache[cacheKey] = results
	se.cacheTimeout[cacheKey] = time.Now()
	se.mu.Unlock()

	elapsed := time.Since(start)
	log.Printf("Search completed in %v, returning %d results", elapsed, len(results))
	return se.limitResults(results, maxResults)
}

func (se *SearchEngine) processAdvancedQuery(query string) []string {
	query = strings.ToLower(query)
	log.Printf("Processing query: '%s'", query)

	if len(query) <= 2 {
		log.Printf("Very short query, using direct matching")
		return []string{query}
	}

	query = se.handleSearchOperators(query)

	phrases := se.extractPhrases(query)
	terms := se.tokenize(query)
	log.Printf("Tokenized terms: %v", terms)

	var allTerms []string
	allTerms = append(allTerms, phrases...)

	for _, term := range terms {
		if !se.stopWords[term] && len(term) > 0 {
			allTerms = append(allTerms, term)

			if len(term) > 2 {
				if synonyms, exists := se.synonyms[term]; exists {
					log.Printf("Found %d synonyms for '%s': %v", len(synonyms), term, synonyms)
					for _, syn := range synonyms {
						if len(syn) > 2 {
							allTerms = append(allTerms, syn)
						}
					}
				}

				corrected := se.spellCorrect(term)
				if corrected != term {
					log.Printf("Spell corrected '%s' to '%s'", term, corrected)
					allTerms = append(allTerms, corrected)
				}

				stemmed := se.stemWord(term)
				if stemmed != term && len(stemmed) > 2 {
					allTerms = append(allTerms, stemmed)
				}
			}
		}
	}

	result := se.removeDuplicates(allTerms)
	log.Printf("Final processed terms: %v", result)
	return result
}

func (se *SearchEngine) extractPhrases(query string) []string {
	var phrases []string

	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindAllStringSubmatch(query, -1)

	for _, match := range matches {
		if len(match) > 1 && len(strings.TrimSpace(match[1])) > 0 {
			phrase := strings.ToLower(strings.TrimSpace(match[1]))
			phrases = append(phrases, phrase)
		}
	}

	return phrases
}

func (se *SearchEngine) tokenize(text string) []string {
	text = strings.ToLower(text)
	var words []string
	var currentWord strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			currentWord.WriteRune(r)
		} else {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
		}
	}

	if currentWord.Len() > 0 {
		words = append(words, currentWord.String())
	}

	return words
}

func (se *SearchEngine) stemWord(word string) string {
	if len(word) <= 2 {
		return word
	}

	suffixes := []string{"ing", "ed", "er", "est", "ly", "tion", "sion", "ness", "ment", "able", "ible", "ous", "ful", "less", "ish", "ive", "al", "ic", "ical", "ate", "ize", "ise", "ity", "ous", "ive"}

	for _, suffix := range suffixes {
		if strings.HasSuffix(word, suffix) && len(word) > len(suffix)+1 {
			return word[:len(word)-len(suffix)]
		}
	}

	if strings.HasSuffix(word, "s") && len(word) > 2 && !strings.HasSuffix(word, "ss") && !strings.HasSuffix(word, "us") {
		return word[:len(word)-1]
	}

	return word
}

func (se *SearchEngine) findAdvancedCandidates(queryTerms []string) map[string]bool {
	candidates := make(map[string]bool)
	termScores := make(map[string]float64)
	requiredTerms := make(map[string]bool)

	log.Printf("Looking for terms: %v", queryTerms)
	log.Printf("Available terms in index: %d", len(se.index.Terms))

	foundTerms := []string{}
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		if len(termLower) < 2 {
			continue
		}

		log.Printf("Searching for term: '%s'", termLower)
		requiredTerms[termLower] = true

		found := false

		if docList, exists := se.index.Terms[termLower]; exists {
			foundTerms = append(foundTerms, termLower)
			log.Printf("Found exact match for '%s' in %d documents", termLower, len(docList))
			weight := se.calculateTermWeight(termLower)
			for _, termFreq := range docList {
				candidates[termFreq.URL] = true
				termScores[termFreq.URL] += weight * termFreq.Score * 2.0
			}
			found = true
		}

		if !found {
			stemmed := se.stemWord(termLower)
			if stemmed != termLower {
				if docList, exists := se.index.Terms[stemmed]; exists {
					foundTerms = append(foundTerms, stemmed)
					log.Printf("Found stemmed match '%s' for '%s' in %d documents", stemmed, termLower, len(docList))
					weight := se.calculateTermWeight(stemmed)
					for _, termFreq := range docList {
						candidates[termFreq.URL] = true
						termScores[termFreq.URL] += weight * termFreq.Score * 1.5
					}
					found = true
				}
			}
		}

		if !found && len(termLower) >= 4 {
			fuzzyMatches := se.findFuzzyMatches(termLower)
			if len(fuzzyMatches) > 0 {
				bestMatch := fuzzyMatches[0]
				if docList, exists := se.index.Terms[bestMatch]; exists {
					foundTerms = append(foundTerms, bestMatch)
					log.Printf("Found fuzzy match '%s' for '%s' in %d documents", bestMatch, termLower, len(docList))
					weight := se.calculateTermWeight(bestMatch)
					for _, termFreq := range docList {
						candidates[termFreq.URL] = true
						termScores[termFreq.URL] += weight * termFreq.Score * 0.8
					}
					found = true
				}
			}
		}
	}

	if len(foundTerms) == 0 {
		log.Printf("No terms found at all")
		return candidates
	}

	if len(queryTerms) > 1 {
		strictCandidates := make(map[string]bool)
		minRequiredTerms := len(queryTerms)

		if len(queryTerms) > 3 {
			minRequiredTerms = int(math.Ceil(float64(len(queryTerms)) * 0.8))
		} else if len(queryTerms) == 3 {
			minRequiredTerms = 2
		}

		for url := range candidates {
			doc, exists := se.index.Docs[url]
			if !exists {
				continue
			}

			titleLower := strings.ToLower(doc.Title)
			contentLower := strings.ToLower(doc.Content)

			termsFoundInDoc := 0
			for _, term := range queryTerms {
				termLower := strings.ToLower(term)
				if strings.Contains(titleLower, termLower) || strings.Contains(contentLower, termLower) {
					termsFoundInDoc++
				}
			}

			if termsFoundInDoc >= minRequiredTerms {
				strictCandidates[url] = true
			}
		}

		log.Printf("Strict filtering: %d candidates remain from %d (required %d/%d terms)",
			len(strictCandidates), len(candidates), minRequiredTerms, len(queryTerms))

		if len(strictCandidates) > 0 {
			return strictCandidates
		}
	}

	return candidates
}

func (se *SearchEngine) calculateTermWeight(term string) float64 {
	if idf, exists := se.idfScores[term]; exists {
		return idf
	}

	docFreq := 0
	if termList, exists := se.index.Terms[term]; exists {
		docFreq = len(termList)
	}

	if docFreq == 0 {
		return 0.1
	}

	totalDocs := float64(se.totalDocs)
	if totalDocs == 0 {
		totalDocs = float64(len(se.index.Docs))
		if totalDocs == 0 {
			return 1.0
		}
	}

	idf := math.Log(totalDocs/float64(docFreq)) + 1.0

	if len(term) > 5 {
		idf *= 1.2
	}

	se.idfScores[term] = idf
	return idf
}

func (se *SearchEngine) scoreAdvancedResults(queryTerms []string, candidates map[string]bool, originalQuery string) []SearchResult {
	var results []SearchResult
	seenTitles := make(map[string]bool)

	for url := range candidates {
		doc, exists := se.index.Docs[url]
		if !exists {
			continue
		}

		if len(strings.TrimSpace(doc.Title)) < 3 || len(strings.TrimSpace(doc.Content)) < 20 {
			continue
		}

		titleKey := strings.ToLower(strings.TrimSpace(doc.Title))
		if seenTitles[titleKey] {
			continue
		}
		seenTitles[titleKey] = true

		relevanceScore := se.calculateIntelligentRelevance(queryTerms, originalQuery, doc)

		if relevanceScore < 0.5 {
			continue
		}

		snippet := se.generateAdvancedSnippet(doc.Content, queryTerms, 200)

		result := SearchResult{
			URL:     doc.URL,
			Title:   doc.Title,
			Snippet: snippet,
			Score:   relevanceScore,
		}

		results = append(results, result)
	}

	return results
}

func (se *SearchEngine) calculateIntelligentRelevance(queryTerms []string, originalQuery string, doc Document) float64 {
	titleLower := strings.ToLower(doc.Title)
	contentLower := strings.ToLower(doc.Content)
	queryLower := strings.ToLower(originalQuery)

	exactQueryInTitle := 0.0
	if strings.Contains(titleLower, queryLower) {
		exactQueryInTitle = 50.0
	}

	exactQueryInContent := 0.0
	if strings.Contains(contentLower, queryLower) {
		exactQueryInContent = 25.0
	}

	termMatchScore := 0.0
	termCount := 0
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		if len(termLower) < 2 {
			continue
		}

		titleMatches := float64(strings.Count(titleLower, termLower))
		contentMatches := float64(strings.Count(contentLower, termLower))

		if titleMatches > 0 {
			termMatchScore += titleMatches * 15.0
			termCount++
		}
		if contentMatches > 0 {
			termMatchScore += contentMatches * 3.0
			termCount++
		}
	}

	termCoverageBonus := 0.0
	if termCount == len(queryTerms) && len(queryTerms) > 1 {
		termCoverageBonus = 20.0
	} else if termCount >= len(queryTerms)/2 {
		termCoverageBonus = 10.0
	}

	semanticScore := se.calculateSemanticRelevance(queryTerms, doc)

	titleQualityScore := se.calculateTitleQuality(doc.Title, queryTerms)

	contentDensityScore := se.calculateContentDensity(queryTerms, doc.Content)

	positionScore := se.calculateTermPositions(queryTerms, doc.Content)

	domainAuthorityScore := se.calculateDomainAuthority(doc.URL)

	totalScore := exactQueryInTitle + exactQueryInContent + termMatchScore +
		termCoverageBonus + semanticScore + titleQualityScore +
		contentDensityScore + positionScore + domainAuthorityScore

	penaltyScore := se.calculateRelevancePenalties(queryTerms, doc)

	finalScore := math.Max(0, totalScore-penaltyScore)

	return finalScore / 10.0
}

func (se *SearchEngine) calculateSemanticRelevance(queryTerms []string, doc Document) float64 {
	score := 0.0
	titleWords := se.tokenize(strings.ToLower(doc.Title))
	contentWords := se.tokenize(strings.ToLower(doc.Content))

	for _, queryTerm := range queryTerms {
		if synonyms, exists := se.synonyms[strings.ToLower(queryTerm)]; exists {
			for _, synonym := range synonyms {
				for _, titleWord := range titleWords {
					if strings.Contains(titleWord, synonym) || strings.Contains(synonym, titleWord) {
						score += 8.0
					}
				}
				for _, contentWord := range contentWords {
					if strings.Contains(contentWord, synonym) || strings.Contains(synonym, contentWord) {
						score += 2.0
					}
				}
			}
		}
	}

	return score
}

func (se *SearchEngine) calculateTitleQuality(title string, queryTerms []string) float64 {
	if len(title) < 5 {
		return -10.0
	}

	titleLower := strings.ToLower(title)

	if strings.Contains(titleLower, "hacker news") && !se.queryContainsHackerNews(queryTerms) {
		return -50.0
	}

	if strings.Contains(titleLower, "news.ycombinator") && !se.queryContainsHackerNews(queryTerms) {
		return -50.0
	}

	genericTerms := []string{"home", "page", "index", "main", "default", "welcome", "news", "blog"}
	for _, generic := range genericTerms {
		if strings.EqualFold(title, generic) {
			return -20.0
		}
	}

	score := 0.0
	if len(title) >= 10 && len(title) <= 100 {
		score += 5.0
	}

	return score
}

func (se *SearchEngine) queryContainsHackerNews(queryTerms []string) bool {
	queryText := strings.ToLower(strings.Join(queryTerms, " "))
	return strings.Contains(queryText, "hacker") || strings.Contains(queryText, "news") || strings.Contains(queryText, "ycombinator")
}

func (se *SearchEngine) calculateContentDensity(queryTerms []string, content string) float64 {
	if len(content) < 50 {
		return -5.0
	}

	contentLower := strings.ToLower(content)
	words := se.tokenize(contentLower)

	if len(words) < 20 {
		return -10.0
	}

	termDensity := 0.0
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		occurrences := float64(strings.Count(contentLower, termLower))
		density := occurrences / float64(len(words))
		termDensity += density * 100.0
	}

	if termDensity > 5.0 {
		return 15.0
	} else if termDensity > 2.0 {
		return 10.0
	} else if termDensity > 0.5 {
		return 5.0
	}

	return 0.0
}

func (se *SearchEngine) calculateTermPositions(queryTerms []string, content string) float64 {
	contentLower := strings.ToLower(content)
	words := se.tokenize(contentLower)

	if len(words) < 10 {
		return 0.0
	}

	score := 0.0

	for _, term := range queryTerms {
		termLower := strings.ToLower(term)

		for i, word := range words {
			if strings.Contains(word, termLower) {
				position := float64(i) / float64(len(words))
				if position < 0.1 {
					score += 8.0
				} else if position < 0.3 {
					score += 5.0
				} else if position < 0.5 {
					score += 2.0
				}
			}
		}
	}

	return score
}

func (se *SearchEngine) calculateDomainAuthority(url string) float64 {
	urlLower := strings.ToLower(url)

	authorityDomains := []string{"wikipedia.org", "github.com", "stackoverflow.com", "reddit.com", "medium.com"}
	for _, domain := range authorityDomains {
		if strings.Contains(urlLower, domain) {
			return 10.0
		}
	}

	if strings.Contains(urlLower, ".edu") || strings.Contains(urlLower, ".gov") {
		return 8.0
	}

	return 0.0
}

func (se *SearchEngine) calculateRelevancePenalties(queryTerms []string, doc Document) float64 {
	penalty := 0.0

	titleLower := strings.ToLower(doc.Title)
	contentLower := strings.ToLower(doc.Content)

	irrelevantPatterns := []string{
		"404", "not found", "error", "coming soon", "under construction",
		"lorem ipsum", "test page", "placeholder", "example",
	}

	for _, pattern := range irrelevantPatterns {
		if strings.Contains(titleLower, pattern) || strings.Contains(contentLower, pattern) {
			penalty += 30.0
		}
	}

	if len(doc.Content) > 10000 {
		queryTermsFound := 0
		for _, term := range queryTerms {
			if strings.Contains(contentLower, strings.ToLower(term)) {
				queryTermsFound++
			}
		}

		if queryTermsFound < len(queryTerms)/2 {
			penalty += 20.0
		}
	}

	if strings.Count(titleLower, " ") > 20 {
		penalty += 10.0
	}

	return penalty
}

func (se *SearchEngine) generateAdvancedSnippet(content string, queryTerms []string, maxLength int) string {
	if content == "" || len(strings.TrimSpace(content)) < 10 {
		return "No content available"
	}

	bestStart := 0
	bestScore := 0.0
	bestLength := 0

	words := strings.Fields(content)
	if len(words) == 0 {
		return "No content available"
	}

	for i := 0; i < len(words); i++ {
		score := 0.0
		length := 0
		wordCount := 0

		for j := i; j < len(words) && wordCount < 40; j++ {
			word := strings.ToLower(words[j])
			length += len(words[j]) + 1
			wordCount++

			for _, term := range queryTerms {
				if strings.Contains(word, term) {
					score += 2.0
					if word == term {
						score += 1.0
					}
				}
			}

			positionBoost := 1.0 / (1.0 + float64(i)/10.0)
			score *= positionBoost
		}

		if score > bestScore && length >= 50 {
			bestScore = score
			bestStart = i
			bestLength = wordCount
		}
	}

	if bestLength == 0 {
		bestLength = min(30, len(words))
	}

	end := min(bestStart+bestLength, len(words))
	snippet := strings.Join(words[bestStart:end], " ")

	if len(snippet) > maxLength {
		words := strings.Fields(snippet)
		truncated := ""
		for _, word := range words {
			if len(truncated)+len(word)+1 <= maxLength-3 {
				if truncated != "" {
					truncated += " "
				}
				truncated += word
			} else {
				break
			}
		}
		snippet = truncated + "..."
	}

	snippet = se.highlightTerms(snippet, queryTerms)
	return snippet
}

func (se *SearchEngine) highlightTerms(text string, queryTerms []string) string {
	result := text

	for _, term := range queryTerms {
		if len(term) > 2 {
			re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(term) + `\b`)
			result = re.ReplaceAllStringFunc(result, func(match string) string {
				return "<b>" + match + "</b>"
			})
		}
	}

	return result
}

func (se *SearchEngine) GetAnalytics() map[string]interface{} {
	se.mu.RLock()
	defer se.mu.RUnlock()

	cacheHitRate := 0.0
	if se.totalQueries > 0 {
		cacheHitRate = float64(se.cacheHits) / float64(se.totalQueries) * 100
	}

	return map[string]interface{}{
		"total_queries":  se.totalQueries,
		"cache_hits":     se.cacheHits,
		"cache_hit_rate": cacheHitRate,
		"unique_queries": len(se.analytics),
	}
}

func (se *SearchEngine) GetAdvancedAnalytics() map[string]interface{} {
	se.mu.RLock()
	defer se.mu.RUnlock()

	cacheHitRate := 0.0
	if se.totalQueries > 0 {
		cacheHitRate = float64(se.cacheHits) / float64(se.totalQueries) * 100
	}

	avgQueryLength := 0.0
	if len(se.analytics) > 0 {
		totalLength := 0
		for query := range se.analytics {
			totalLength += len(strings.Fields(query))
		}
		avgQueryLength = float64(totalLength) / float64(len(se.analytics))
	}

	return map[string]interface{}{
		"total_queries":    se.totalQueries,
		"cache_hits":       se.cacheHits,
		"cache_hit_rate":   cacheHitRate,
		"unique_queries":   len(se.analytics),
		"avg_query_length": avgQueryLength,
		"cache_size":       len(se.queryCache),
		"total_documents":  se.totalDocs,
		"avg_doc_length":   se.avgDocLength,
		"index_size":       len(se.index.Terms),
	}
}

func (se *SearchEngine) GetQueryTrends() map[string]interface{} {
	se.mu.RLock()
	defer se.mu.RUnlock()

	type queryFreq struct {
		Query string
		Count int
	}

	var queries []queryFreq
	for query, count := range se.analytics {
		queries = append(queries, queryFreq{Query: query, Count: count})
	}

	sort.Slice(queries, func(i, j int) bool {
		return queries[i].Count > queries[j].Count
	})

	topQueries := make([]map[string]interface{}, 0, 10)
	for i, q := range queries {
		if i >= 10 {
			break
		}
		topQueries = append(topQueries, map[string]interface{}{
			"query": q.Query,
			"count": q.Count,
		})
	}

	return map[string]interface{}{
		"top_queries":          topQueries,
		"total_unique_queries": len(queries),
	}
}

func (se *SearchEngine) UpdateDocumentStats() {
	if se.index == nil {
		return
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	totalLength := 0
	docCount := 0

	for _, termList := range se.index.Terms {
		for _, termFreq := range termList {
			if length, exists := se.docLengths[termFreq.URL]; exists {
				totalLength += length
			} else {
				docLength := int(termFreq.Score * 100)
				se.docLengths[termFreq.URL] = docLength
				totalLength += docLength
			}
			docCount++
		}
	}

	if docCount > 0 {
		se.avgDocLength = float64(totalLength) / float64(docCount)
	}
	se.totalDocs = docCount
}

func (se *SearchEngine) ClearOldCache() {
	se.mu.Lock()
	defer se.mu.Unlock()

	now := time.Now()
	for key, timestamp := range se.cacheTimeout {
		if now.Sub(timestamp) > 10*time.Minute {
			delete(se.queryCache, key)
			delete(se.cacheTimeout, key)
		}
	}
}

func (se *SearchEngine) backgroundMaintenance() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		se.ClearOldCache()
		se.UpdateDocumentStats()
		log.Printf("Background maintenance completed - cache size: %d", len(se.queryCache))
	}
}

func (se *SearchEngine) spellCorrect(term string) string {
	if len(term) < 4 {
		return term
	}

	bestMatch := term
	minDistance := 2

	for indexTerm := range se.index.Terms {
		if len(indexTerm) >= len(term)-1 && len(indexTerm) <= len(term)+1 {
			distance := se.editDistance(term, indexTerm)
			if distance < minDistance && distance > 0 {
				minDistance = distance
				bestMatch = indexTerm
			}
		}
	}

	return bestMatch
}

func (se *SearchEngine) findFuzzyMatches(term string) []string {
	var matches []string
	if len(term) < 3 {
		return matches
	}

	for indexTerm := range se.index.Terms {
		if len(indexTerm) >= len(term)-1 && len(indexTerm) <= len(term)+1 {
			distance := se.editDistance(term, indexTerm)
			if distance <= 1 {
				matches = append(matches, indexTerm)
				if len(matches) >= 3 {
					break
				}
			}
		}
	}

	return matches
}

func (se *SearchEngine) editDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	if s1[0] == s2[0] {
		return se.editDistance(s1[1:], s2[1:])
	}

	insert := se.editDistance(s1, s2[1:])
	delete := se.editDistance(s1[1:], s2)
	replace := se.editDistance(s1[1:], s2[1:])

	min := insert
	if delete < min {
		min = delete
	}
	if replace < min {
		min = replace
	}

	return 1 + min
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (se *SearchEngine) handleSearchOperators(query string) string {
	query = strings.ReplaceAll(query, " and ", " ")
	query = strings.ReplaceAll(query, " or ", " ")
	query = strings.ReplaceAll(query, " not ", " -")
	query = strings.ReplaceAll(query, "+", "")

	return query
}

func (se *SearchEngine) GetTopQueries(limit int) []map[string]interface{} {
	type queryCount struct {
		query string
		count int
	}

	var queries []queryCount
	for query, count := range se.analytics {
		queries = append(queries, queryCount{query, count})
	}

	sort.Slice(queries, func(i, j int) bool {
		return queries[i].count > queries[j].count
	})

	var result []map[string]interface{}
	for i, q := range queries {
		if i >= limit {
			break
		}
		result = append(result, map[string]interface{}{
			"query": q.query,
			"count": q.count,
		})
	}

	return result
}

func (se *SearchEngine) removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !keys[item] && len(item) > 1 {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}

func (se *SearchEngine) limitResults(results []SearchResult, maxResults int) []SearchResult {
	if len(results) <= maxResults {
		return results
	}
	return results[:maxResults]
}

func (se *SearchEngine) optimizedSort(results []SearchResult) {
	if len(results) <= 1 {
		return
	}

	if len(results) < 50 {
		sort.Slice(results, func(i, j int) bool {
			if math.Abs(results[i].Score-results[j].Score) < 0.001 {
				return len(results[i].Title) < len(results[j].Title)
			}
			return results[i].Score > results[j].Score
		})
		return
	}

	se.quickSortResults(results, 0, len(results)-1)
}

func (se *SearchEngine) quickSortResults(results []SearchResult, low, high int) {
	if low < high {
		pi := se.partitionResults(results, low, high)
		se.quickSortResults(results, low, pi-1)
		se.quickSortResults(results, pi+1, high)
	}
}

func (se *SearchEngine) partitionResults(results []SearchResult, low, high int) int {
	pivot := results[high]
	i := low - 1

	for j := low; j < high; j++ {
		if results[j].Score > pivot.Score ||
			(math.Abs(results[j].Score-pivot.Score) < 0.001 && len(results[j].Title) < len(pivot.Title)) {
			i++
			results[i], results[j] = results[j], results[i]
		}
	}

	results[i+1], results[high] = results[high], results[i+1]
	return i + 1
}
