package main

import (
	"bufio"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

type SearchSuggestion struct {
	Text      string  `json:"text"`
	Score     float64 `json:"score"`
	Frequency int     `json:"frequency"`
	Type      string  `json:"type"`
}

type TrieNode struct {
	children map[rune]*TrieNode
}

type AutoComplete struct {
	root           *TrieNode
	maxSuggestions int
	trending       []string
	lastUpdated    time.Time
}

type SearchEngine struct {
	index        *InvertedIndex
	mu           sync.RWMutex
	stopWords    map[string]struct{}
	synonyms     map[string][]string
	queryCache   sync.Map
	cacheTimeout sync.Map
	idfScores    sync.Map
	totalDocs    int64
	cacheHits    int64
	totalQueries int64
	termCounts   sync.Map
	docLengths   sync.Map
	avgDocLength float64
	autoComplete *AutoComplete
	popularTerms sync.Map
	phraseRegex  *regexp.Regexp
	commonWords  map[string]struct{}
}

type QueryLogEntry struct {
	Query     string    `json:"query"`
	Timestamp time.Time `json:"timestamp"`
	Results   int       `json:"results"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
}

type CacheItem struct {
	Results   []SearchResult
	Timestamp time.Time
}

func CreateSearchEngine() *SearchEngine {
	stopWords := loadStopWordsFromFile()
	synonyms := loadSynonymsFromFile()
	commonWords := createCommonWordsMap()

	engine := &SearchEngine{
		stopWords:    stopWords,
		synonyms:     synonyms,
		avgDocLength: 0,
		autoComplete: CreateAutoComplete(),
		phraseRegex:  regexp.MustCompile(`"([^"]+)"`),
		commonWords:  commonWords,
	}

	go engine.backgroundMaintenance()

	return engine
}

func CreateAutoComplete() *AutoComplete {
	return &AutoComplete{
		root: &TrieNode{
			children: make(map[rune]*TrieNode),
		},
		maxSuggestions: 10,
		trending:       make([]string, 0),
		lastUpdated:    time.Now(),
	}
}

func loadStopWordsFromFile() map[string]struct{} {
	stopWords := make(map[string]struct{})

	file, err := os.Open("data/stopwords.txt")
	if err != nil {
		return stopWords
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" {
			stopWords[word] = struct{}{}
		}
	}

	return stopWords
}

func loadSynonymsFromFile() map[string][]string {
	synonyms := make(map[string][]string)

	file, err := os.Open("data/synonyms.txt")
	if err != nil {
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

	return synonyms
}

func createCommonWordsMap() map[string]struct{} {
	words := []string{
		"does", "doesn", "what", "when", "where", "why", "how", "who", "which",
		"that", "this", "they", "them", "their", "there", "here", "were", "are",
		"was", "will", "would", "could", "should", "can", "has", "have", "had",
		"been", "being", "get", "got", "take", "took", "make", "made", "come",
		"came", "go", "went", "see", "saw", "know", "knew", "think", "thought",
		"say", "said", "tell", "told", "give", "gave", "find", "found", "work",
		"worked", "place", "time", "year", "day", "way",
	}

	result := make(map[string]struct{}, len(words))
	for _, word := range words {
		result[word] = struct{}{}
	}
	return result
}

func (se *SearchEngine) LoadIndex(index *InvertedIndex) {
	se.mu.Lock()
	se.index = index

	count := int64(0)
	index.Docs.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	atomic.StoreInt64(&se.totalDocs, count)
	se.mu.Unlock()

	se.computeIDF()
}

func (se *SearchEngine) computeIDF() {
	totalDocs := atomic.LoadInt64(&se.totalDocs)
	se.index.Terms.Range(func(key, value interface{}) bool {
		term := key.(string)
		termFreqs := value.([]TermFreq)
		df := len(termFreqs)
		if df > 0 {
			idf := math.Log(float64(totalDocs) / float64(df))
			se.idfScores.Store(term, idf)
		}
		return true
	})
}

func (se *SearchEngine) Search(query string, limit int) ([]SearchResult, string) {
	start := time.Now()
	results := se.SearchWithLimit(query, limit)
	return results, time.Since(start).String()
}

func (se *SearchEngine) SearchPaginated(query string, page, limit int) ([]SearchResult, int, string) {
	start := time.Now()
	atomic.AddInt64(&se.totalQueries, 1)

	if se.index == nil {
		return nil, 0, time.Since(start).String()
	}

	cacheKey := strings.ToLower(query)
	var allResults []SearchResult

	if cached, exists := se.queryCache.Load(cacheKey); exists {
		cacheItem := cached.(CacheItem)
		if time.Since(cacheItem.Timestamp) < 5*time.Minute {
			atomic.AddInt64(&se.cacheHits, 1)
			allResults = cacheItem.Results
		} else {
			allResults = se.SearchWithLimit(query, 10000)
		}
	} else {
		allResults = se.SearchWithLimit(query, 10000)
	}

	total := len(allResults)
	offset := (page - 1) * limit
	if offset >= total {
		return []SearchResult{}, total, time.Since(start).String()
	}

	end := offset + limit
	if end > total {
		end = total
	}

	paginatedResults := make([]SearchResult, end-offset)
	copy(paginatedResults, allResults[offset:end])

	for i := range paginatedResults {
		paginatedResults[i].Rank = offset + i + 1
	}

	return paginatedResults, total, time.Since(start).String()
}

func (se *SearchEngine) SearchWithLimit(query string, maxResults int) []SearchResult {
	atomic.AddInt64(&se.totalQueries, 1)

	if se.index == nil {
		return nil
	}

	cacheKey := strings.ToLower(query)

	if cached, exists := se.queryCache.Load(cacheKey); exists {
		cacheItem := cached.(CacheItem)
		if time.Since(cacheItem.Timestamp) < 5*time.Minute {
			atomic.AddInt64(&se.cacheHits, 1)
			return cacheItem.Results
		}
	}

	queryTerms := se.processQuery(query)
	candidates := se.findCandidates(queryTerms)

	if len(candidates) == 0 && len(queryTerms) == 1 {
		singleTerm := queryTerms[0]
		se.index.Terms.Range(func(key, value interface{}) bool {
			indexTerm := key.(string)
			if strings.Contains(indexTerm, singleTerm) || strings.Contains(singleTerm, indexTerm) {
				docList := value.([]TermFreq)
				if len(docList) > 0 {
					for _, termFreq := range docList {
						candidates[termFreq.URL] = true
					}
					if len(candidates) >= 20 {
						return false
					}
				}
			}
			return true
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	results := se.scoreResults(queryTerms, candidates, query)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	for i := range results {
		results[i].Rank = i + 1
	}

	se.queryCache.Store(cacheKey, CacheItem{
		Results:   results,
		Timestamp: time.Now(),
	})

	return results
}

func (se *SearchEngine) processQuery(query string) []string {
	query = strings.ToLower(query)

	if len(query) <= 2 {
		return []string{query}
	}

	query = se.handleSearchOperators(query)
	phrases := se.extractPhrases(query)
	terms := se.tokenize(query)

	var allTerms []string
	allTerms = append(allTerms, phrases...)

	for _, term := range terms {
		if _, exists := se.stopWords[term]; !exists && len(term) > 0 {
			allTerms = append(allTerms, term)

			if len(term) > 2 {
				if synonyms, exists := se.synonyms[term]; exists {
					for _, syn := range synonyms {
						if len(syn) > 2 {
							allTerms = append(allTerms, syn)
						}
					}
				}

				corrected := se.spellCorrect(term)
				if corrected != term {
					allTerms = append(allTerms, corrected)
				}

				stemmed := se.stemWord(term)
				if stemmed != term && len(stemmed) > 2 {
					allTerms = append(allTerms, stemmed)
				}
			}
		}
	}

	return allTerms
}

func (se *SearchEngine) extractPhrases(query string) []string {
	var phrases []string
	matches := se.phraseRegex.FindAllStringSubmatch(query, -1)

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

func (se *SearchEngine) findCandidates(queryTerms []string) map[string]bool {
	candidates := make(map[string]bool)
	termScores := make(map[string]float64)
	requiredTerms := make(map[string]bool)

	foundTerms := []string{}
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		if len(termLower) < 2 {
			continue
		}

		requiredTerms[termLower] = true
		found := false

		if docListInterface, exists := se.index.Terms.Load(termLower); exists {
			docList := docListInterface.([]TermFreq)
			foundTerms = append(foundTerms, termLower)
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
				if docListInterface, exists := se.index.Terms.Load(stemmed); exists {
					docList := docListInterface.([]TermFreq)
					foundTerms = append(foundTerms, stemmed)
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
				if docListInterface, exists := se.index.Terms.Load(bestMatch); exists {
					docList := docListInterface.([]TermFreq)
					foundTerms = append(foundTerms, bestMatch)
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
			docInterface, exists := se.index.Docs.Load(url)
			if !exists {
				continue
			}
			doc := docInterface.(Document)

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

		if len(strictCandidates) > 0 {
			return strictCandidates
		}
	}

	return candidates
}

func (se *SearchEngine) calculateTermWeight(term string) float64 {
	if idf, exists := se.idfScores.Load(term); exists {
		return idf.(float64)
	}

	docFreq := 0
	if termListInterface, exists := se.index.Terms.Load(term); exists {
		termList := termListInterface.([]TermFreq)
		docFreq = len(termList)
	}

	if docFreq == 0 {
		return 0.1
	}

	totalDocs := float64(atomic.LoadInt64(&se.totalDocs))
	if totalDocs == 0 {
		count := int64(0)
		se.index.Docs.Range(func(key, value interface{}) bool {
			count++
			return true
		})
		totalDocs = float64(count)
		if totalDocs == 0 {
			return 1.0
		}
	}

	idf := math.Log(totalDocs/float64(docFreq)) + 1.0

	if len(term) > 5 {
		idf *= 1.2
	}

	se.idfScores.Store(term, idf)
	return idf
}

func (se *SearchEngine) scoreResults(queryTerms []string, candidates map[string]bool, originalQuery string) []SearchResult {
	var results []SearchResult
	seenTitles := make(map[string]bool)

	for url := range candidates {
		docInterface, exists := se.index.Docs.Load(url)
		if !exists {
			continue
		}
		doc := docInterface.(Document)

		if len(strings.TrimSpace(doc.Title)) < 3 || len(strings.TrimSpace(doc.Content)) < 20 {
			continue
		}

		titleKey := strings.ToLower(strings.TrimSpace(doc.Title))
		if seenTitles[titleKey] {
			continue
		}
		seenTitles[titleKey] = true

		relevanceScore := se.calculateRelevance(queryTerms, originalQuery, doc)

		if relevanceScore < 0.1 {
			continue
		}

		snippet := se.generateSnippet(doc.Content, queryTerms, 200)

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

func (se *SearchEngine) calculateRelevance(queryTerms []string, originalQuery string, doc Document) float64 {
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

	contentQualityScore := se.calculateContentQuality(doc)

	totalScore := exactQueryInTitle + exactQueryInContent + termMatchScore +
		termCoverageBonus + semanticScore + titleQualityScore +
		contentDensityScore + positionScore + contentQualityScore

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

func (se *SearchEngine) calculateContentQuality(doc Document) float64 {
	qualityScore := 0.0

	contentLength := len(doc.Content)
	if contentLength > 5000 {
		qualityScore += 8.0
	} else if contentLength > 2000 {
		qualityScore += 6.0
	} else if contentLength > 1000 {
		qualityScore += 4.0
	} else if contentLength > 500 {
		qualityScore += 2.0
	}

	titleLength := len(doc.Title)
	if titleLength > 10 && titleLength < 100 {
		qualityScore += 3.0
	}

	words := strings.Fields(strings.ToLower(doc.Content))
	uniqueWords := make(map[string]bool)
	for _, word := range words {
		if len(word) > 3 {
			uniqueWords[word] = true
		}
	}

	if len(words) > 0 {
		diversityRatio := float64(len(uniqueWords)) / float64(len(words))
		if diversityRatio > 0.4 {
			qualityScore += 5.0
		} else if diversityRatio > 0.3 {
			qualityScore += 3.0
		} else if diversityRatio > 0.2 {
			qualityScore += 1.0
		}
	}

	contentLower := strings.ToLower(doc.Content)
	structureIndicators := []string{
		"introduction", "overview", "definition", "explanation",
		"background", "history", "characteristics", "features",
		"description", "summary", "conclusion",
	}

	structureCount := 0
	for _, indicator := range structureIndicators {
		if strings.Contains(contentLower, indicator) {
			structureCount++
		}
	}
	qualityScore += float64(structureCount) * 0.5

	lowQualityPatterns := []string{
		"click here", "buy now", "limited time", "advertisement",
		"popup", "loading", "javascript required", "cookies",
	}

	penaltyCount := 0
	for _, pattern := range lowQualityPatterns {
		if strings.Contains(contentLower, pattern) {
			penaltyCount++
		}
	}
	qualityScore -= float64(penaltyCount) * 2.0

	return math.Max(0, qualityScore)
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

func (se *SearchEngine) generateSnippet(content string, queryTerms []string, maxLength int) string {
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
						score += 5.0
					}
				}
			}
		}

		positionBoost := 1.0
		if i < 10 {
			positionBoost = 1.5
		} else if i < 20 {
			positionBoost = 1.2
		}

		score *= positionBoost

		if score > bestScore && length >= 50 {
			bestScore = score
			bestStart = i
		}
	}

	end := min(bestStart+bestLength, len(words))
	snippet := strings.Join(words[bestStart:end], " ")
	if len(snippet) > maxLength {
		truncated := ""
		words := strings.Fields(snippet)
		for _, word := range words {
			truncated += word + " "
			if len(truncated) >= maxLength {
				truncated = strings.TrimSpace(truncated)
				snippet = truncated + "..."
				break
			}
		}
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

func (se *SearchEngine) GetSuggestions(query string) []string {
	return nil
}

func (se *SearchEngine) GetAnalytics() interface{} {
	return nil
}

func (se *SearchEngine) GetQueryTrends() interface{} {
	return nil
}

func (se *SearchEngine) GetTopQueries(limit int) []map[string]interface{} {
	return nil
}

func (se *SearchEngine) UpdateDocumentStats() {
	return
}

func (se *SearchEngine) ClearOldCache() {
	return
}

func (se *SearchEngine) backgroundMaintenance() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		se.ClearOldCache()
		se.UpdateDocumentStats()
	}
}

func (se *SearchEngine) spellCorrect(term string) string {
	if len(term) < 4 {
		return term
	}

	if _, exists := se.commonWords[term]; exists {
		return term
	}

	return term
}

func (se *SearchEngine) findFuzzyMatches(term string) []string {
	var matches []string
	if len(term) < 3 {
		return matches
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

func (se *SearchEngine) handleSearchOperators(query string) string {
	query = strings.ReplaceAll(query, " and ", " ")
	query = strings.ReplaceAll(query, " or ", " ")
	query = strings.ReplaceAll(query, " not ", " -")
	query = strings.ReplaceAll(query, "+", "")

	return query
}

func (se *SearchEngine) GetCurrentIndex() *InvertedIndex {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.index
}
