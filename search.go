package main

import (
	"bufio"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
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
	stopWords    map[string]bool
	pageRank     map[string]float64
	linkGraph    map[string][]string
	synonyms     map[string][]string
	queryCache   map[string][]SearchResult
	idfScores    map[string]float64
	totalDocs    int
	analytics    map[string]int
	cacheHits    int
	totalQueries int
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
		idfScores:    make(map[string]float64),
		totalDocs:    0,
		analytics:    make(map[string]int),
		cacheHits:    0,
		totalQueries: 0,
	}

	log.Printf("Search engine initialized with %d stop words and %d synonym groups", len(stopWords), len(synonyms))
	return engine
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

	if cached, exists := se.queryCache[cacheKey]; exists {
		se.cacheHits++
		log.Printf("Cache hit for query: '%s' (hit rate: %.1f%%)", query, float64(se.cacheHits)/float64(se.totalQueries)*100)
		allResults = cached
	} else {
		se.analytics[query]++
		allResults = se.SearchAdvanced(query, 10000) // Get all results for proper pagination

		// Cache the results
		if len(se.queryCache) > 1000 {
			se.queryCache = make(map[string][]SearchResult)
		}
		se.queryCache[cacheKey] = allResults
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
	if cached, exists := se.queryCache[cacheKey]; exists {
		se.cacheHits++
		log.Printf("Cache hit for query: '%s' (hit rate: %.1f%%)", query, float64(se.cacheHits)/float64(se.totalQueries)*100)
		return se.limitResults(cached, maxResults)
	}

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

	sort.Slice(results, func(i, j int) bool {
		if math.Abs(results[i].Score-results[j].Score) < 0.001 {
			return len(results[i].Title) < len(results[j].Title)
		}
		return results[i].Score > results[j].Score
	})

	for i := range results {
		results[i].Rank = i + 1
	}

	if len(se.queryCache) > 1000 {
		se.queryCache = make(map[string][]SearchResult)
	}

	se.queryCache[cacheKey] = results

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

	log.Printf("Looking for terms: %v", queryTerms)
	log.Printf("Available terms in index: %d", len(se.index.Terms))

	foundTerms := []string{}
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		log.Printf("Searching for term: '%s'", termLower)

		found := false

		if docList, exists := se.index.Terms[termLower]; exists {
			foundTerms = append(foundTerms, termLower)
			log.Printf("Found exact match for '%s' in %d documents", termLower, len(docList))
			weight := se.calculateTermWeight(termLower)
			for _, termFreq := range docList {
				candidates[termFreq.URL] = true
				termScores[termFreq.URL] += weight * termFreq.Score
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
						termScores[termFreq.URL] += weight * termFreq.Score * 0.9
					}
					found = true
				}
			}
		}

		if !found {
			log.Printf("No exact match for '%s', checking partial matches", termLower)
			partialCount := 0

			for indexTerm := range se.index.Terms {
				if (strings.Contains(indexTerm, termLower) || strings.Contains(termLower, indexTerm)) && len(indexTerm) > 2 {
					if docList := se.index.Terms[indexTerm]; len(docList) > 0 {
						foundTerms = append(foundTerms, indexTerm)
						log.Printf("Found partial match '%s' for '%s' in %d documents", indexTerm, termLower, len(docList))
						weight := se.calculateTermWeight(indexTerm)
						for _, termFreq := range docList {
							candidates[termFreq.URL] = true
							termScores[termFreq.URL] += weight * termFreq.Score * 0.6
						}
						partialCount++
						if partialCount >= 5 {
							break
						}
					}
				}
			}

			if partialCount == 0 {
				pluralTerm := termLower + "s"
				if docList, exists := se.index.Terms[pluralTerm]; exists {
					foundTerms = append(foundTerms, pluralTerm)
					log.Printf("Found plural match '%s' for '%s' in %d documents", pluralTerm, termLower, len(docList))
					weight := se.calculateTermWeight(pluralTerm)
					for _, termFreq := range docList {
						candidates[termFreq.URL] = true
						termScores[termFreq.URL] += weight * termFreq.Score * 0.8
					}
					partialCount++
				}

				if strings.HasSuffix(termLower, "s") && len(termLower) > 3 {
					singularTerm := termLower[:len(termLower)-1]
					if docList, exists := se.index.Terms[singularTerm]; exists {
						foundTerms = append(foundTerms, singularTerm)
						log.Printf("Found singular match '%s' for '%s' in %d documents", singularTerm, termLower, len(docList))
						weight := se.calculateTermWeight(singularTerm)
						for _, termFreq := range docList {
							candidates[termFreq.URL] = true
							termScores[termFreq.URL] += weight * termFreq.Score * 0.8
						}
						partialCount++
					}
				}
			}

			if partialCount == 0 {
				fuzzyMatches := se.findFuzzyMatches(termLower)
				for _, fuzzyTerm := range fuzzyMatches {
					if docList, exists := se.index.Terms[fuzzyTerm]; exists {
						foundTerms = append(foundTerms, fuzzyTerm)
						log.Printf("Found fuzzy match '%s' for '%s' in %d documents", fuzzyTerm, termLower, len(docList))
						weight := se.calculateTermWeight(fuzzyTerm)
						for _, termFreq := range docList {
							candidates[termFreq.URL] = true
							termScores[termFreq.URL] += weight * termFreq.Score * 0.4
						}
						partialCount++
						if partialCount >= 2 {
							break
						}
					}
				}
			}

			if partialCount == 0 {
				log.Printf("No matches found for term '%s'", termLower)
			}
		}
	}
	log.Printf("Found matching terms: %v", foundTerms)

	if len(candidates) > 100 {
		threshold := se.calculateThreshold(termScores) * 0.4
		log.Printf("Applying lowered threshold %.2f to %d candidates", threshold, len(candidates))

		filteredCandidates := make(map[string]bool)
		for url := range candidates {
			if termScores[url] >= threshold {
				filteredCandidates[url] = true
			}
		}
		log.Printf("After threshold filtering: %d candidates remain", len(filteredCandidates))
		return filteredCandidates
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

func (se *SearchEngine) calculateThreshold(termScores map[string]float64) float64 {
	if len(termScores) == 0 {
		return 0
	}

	var scores []float64
	for _, score := range termScores {
		scores = append(scores, score)
	}

	sort.Float64s(scores)

	if len(scores) < 10 {
		return 0
	}

	percentile := int(float64(len(scores)) * 0.2)
	return scores[percentile]
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

		titleScore := se.calculateTextScore(queryTerms, doc.Title, 6.0)
		contentScore := se.calculateTextScore(queryTerms, doc.Content, 1.0)
		urlScore := se.calculateURLScore(queryTerms, doc.URL)

		phraseScore := se.calculatePhraseScore(originalQuery, doc.Content, doc.Title)
		queryMatchScore := se.calculateQueryMatchScore(queryTerms, doc.Title, doc.Content)

		titleMatchBonus := 0.0
		titleLower := strings.ToLower(doc.Title)
		for _, term := range queryTerms {
			if strings.Contains(titleLower, strings.ToLower(term)) {
				titleMatchBonus += 2.0
			}
		}

		exactTitleMatchBonus := 0.0
		fullQuery := strings.Join(queryTerms, " ")
		if strings.Contains(titleLower, strings.ToLower(fullQuery)) {
			exactTitleMatchBonus = 8.0
		}

		contentQuality := se.calculateContentQuality(doc.Content, queryTerms)

		totalScore := (titleScore*0.45 + contentScore*0.3 + urlScore*0.08 +
			phraseScore*0.12 + queryMatchScore*0.03 + titleMatchBonus*0.015 +
			exactTitleMatchBonus*0.005 + contentQuality*0.02)

		if totalScore > 0.1 {
			snippet := se.generateAdvancedSnippet(doc.Content, queryTerms, 200)

			result := SearchResult{
				URL:     doc.URL,
				Title:   doc.Title,
				Snippet: snippet,
				Score:   totalScore,
			}

			results = append(results, result)
		}
	}

	return results
}

func (se *SearchEngine) calculateTextScore(queryTerms []string, text string, weight float64) float64 {
	if text == "" {
		return 0
	}

	textLower := strings.ToLower(text)
	words := se.tokenize(textLower)
	wordCount := make(map[string]int)

	for _, word := range words {
		wordCount[word]++
	}

	score := 0.0
	totalWords := float64(len(words))
	totalQueryTermsFound := 0
	if totalWords == 0 {
		totalWords = 1
	}

	k1 := 1.2
	b := 0.75
	avgDocLength := 150.0

	exactPhraseBonus := 0.0
	fullQuery := strings.Join(queryTerms, " ")
	if strings.Contains(textLower, fullQuery) {
		exactPhraseBonus = 5.0 * weight
	}

	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		if count, exists := wordCount[termLower]; exists {
			totalQueryTermsFound++
			tf := float64(count)

			idf := se.calculateTermWeight(termLower)
			if idf == 0 {
				idf = 1.0
			}

			bm25 := idf * (tf * (k1 + 1)) / (tf + k1*(1-b+b*(totalWords/avgDocLength)))

			positionBoost := se.calculatePositionBoost(termLower, textLower)

			termLength := len(termLower)
			lengthBonus := 1.0
			if termLength > 6 {
				lengthBonus = 1.5
			} else if termLength > 4 {
				lengthBonus = 1.2
			}

			termScore := bm25 * positionBoost * lengthBonus
			score += termScore
		}
	}

	queryTermCoverage := float64(totalQueryTermsFound) / float64(len(queryTerms))
	if queryTermCoverage < 0.5 && len(queryTerms) > 1 {
		score *= 0.3
	}

	docLengthNormalization := 1.0
	if totalWords > 1000 {
		docLengthNormalization = 0.8
	} else if totalWords > 500 {
		docLengthNormalization = 0.9
	} else if totalWords < 50 {
		docLengthNormalization = 0.7
	}

	return (score + exactPhraseBonus) * weight * docLengthNormalization
}

func (se *SearchEngine) calculatePositionBoost(term, text string) float64 {
	index := strings.Index(text, term)
	if index == -1 {
		return 1.0
	}

	textLen := float64(len(text))
	if textLen == 0 {
		return 1.0
	}

	position := float64(index) / textLen

	if position < 0.02 {
		return 5.0
	} else if position < 0.05 {
		return 4.0
	} else if position < 0.1 {
		return 3.0
	} else if position < 0.2 {
		return 2.5
	} else if position < 0.3 {
		return 2.0
	} else if position < 0.5 {
		return 1.5
	}

	return 1.0
}

func (se *SearchEngine) calculateURLScore(queryTerms []string, url string) float64 {
	urlLower := strings.ToLower(url)
	score := 0.0

	for _, term := range queryTerms {
		if strings.Contains(urlLower, term) {
			pathParts := strings.Split(urlLower, "/")
			for _, part := range pathParts {
				if strings.Contains(part, term) {
					if part == term {
						score += 4.0
					} else if strings.HasPrefix(part, term) || strings.HasSuffix(part, term) {
						score += 3.0
					} else {
						score += 2.0
					}
				}
			}
		}
	}

	if strings.Contains(urlLower, "https") {
		score += 0.2
	}

	depth := strings.Count(urlLower, "/") - 2
	if depth < 2 {
		score += 1.0
	} else if depth < 4 {
		score += 0.5
	}

	return score
}

func (se *SearchEngine) calculatePhraseScore(originalQuery, content, title string) float64 {
	phrases := se.extractPhrases(originalQuery)
	score := 0.0
	contentLower := strings.ToLower(content)
	titleLower := strings.ToLower(title)

	for _, phrase := range phrases {
		if strings.Contains(titleLower, phrase) {
			score += 12.0
		}
		if strings.Contains(contentLower, phrase) {
			score += 6.0
		}
	}

	queryLower := strings.ToLower(originalQuery)
	if !strings.Contains(queryLower, "\"") {
		words := strings.Fields(queryLower)
		if len(words) > 1 {
			fullQuery := strings.Join(words, " ")
			if strings.Contains(titleLower, fullQuery) {
				score += 8.0
			}
			if strings.Contains(contentLower, fullQuery) {
				score += 4.0
			}
		}
	}

	return score
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

func (se *SearchEngine) splitIntoSentences(text string) []string {
	re := regexp.MustCompile(`[.!?]+\s+`)
	sentences := re.Split(text, -1)

	var result []string
	for _, sentence := range sentences {
		trimmed := strings.TrimSpace(sentence)
		if len(trimmed) > 15 && len(trimmed) < 300 {
			result = append(result, trimmed)
		}
	}

	return result
}

func (se *SearchEngine) findBestSentences(sentences []string, queryTerms []string, maxSentences int) []string {
	type sentenceScore struct {
		sentence string
		score    float64
		index    int
	}

	var scored []sentenceScore

	for i, sentence := range sentences {
		score := 0.0
		sentenceLower := strings.ToLower(sentence)

		for _, term := range queryTerms {
			if strings.Contains(sentenceLower, term) {
				score += 2.0

				termCount := strings.Count(sentenceLower, term)
				score += float64(termCount-1) * 1.0
			}
		}

		if score > 0 {
			scored = append(scored, sentenceScore{
				sentence: sentence,
				score:    score,
				index:    i,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].index < scored[j].index
		}
		return scored[i].score > scored[j].score
	})

	var result []string
	count := 0
	for _, s := range scored {
		if count >= maxSentences {
			break
		}
		result = append(result, s.sentence)
		count++
	}

	if len(result) == 0 && len(sentences) > 0 {
		result = append(result, sentences[0])
	}

	return result
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
	hitRate := 0.0
	if se.totalQueries > 0 {
		hitRate = float64(se.cacheHits) / float64(se.totalQueries) * 100
	}

	return map[string]interface{}{
		"total_queries": se.totalQueries,
		"cache_hits":    se.cacheHits,
		"hit_rate":      hitRate,
		"cache_size":    len(se.queryCache),
		"top_queries":   se.GetTopQueries(10),
	}
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

func (se *SearchEngine) calculateQueryMatchScore(queryTerms []string, title, content string) float64 {
	titleLower := strings.ToLower(title)
	contentLower := strings.ToLower(content)

	matchCount := 0
	totalTerms := len(queryTerms)

	for _, term := range queryTerms {
		if strings.Contains(titleLower, term) || strings.Contains(contentLower, term) {
			matchCount++
		}
	}

	if totalTerms == 0 {
		return 0
	}

	return float64(matchCount) / float64(totalTerms)
}

func loadStopWordsFromFile() map[string]bool {
	stopWords := make(map[string]bool)

	file, err := os.Open("data/stopwords.txt")
	if err != nil {
		log.Printf("Could not load stopwords.txt, using defaults: %v", err)
		return map[string]bool{
			"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
			"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
			"with": true, "by": true,
		}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(strings.ToLower(scanner.Text()))
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
		log.Printf("Failed to open synonyms file: %v", err)
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

func (se *SearchEngine) calculateContentQuality(content string, queryTerms []string) float64 {
	words := se.tokenize(content)
	if len(words) < 10 {
		return 0.1
	}

	wordCount := make(map[string]int)
	for _, word := range words {
		wordCount[word]++
	}

	uniqueWords := len(wordCount)
	totalWords := len(words)
	diversity := float64(uniqueWords) / float64(totalWords)

	queryRelevance := 0.0
	for _, term := range queryTerms {
		if count, exists := wordCount[term]; exists {
			queryRelevance += float64(count)
		}
	}

	queryDensity := queryRelevance / float64(totalWords)

	if queryDensity > 0.1 {
		queryDensity = 0.1
	}

	lengthScore := 1.0
	if totalWords < 50 {
		lengthScore = 0.5
	} else if totalWords > 1000 {
		lengthScore = 0.8
	}

	return diversity*0.3 + queryDensity*5.0 + lengthScore*0.2
}

func (se *SearchEngine) handleSearchOperators(query string) string {
	query = strings.ReplaceAll(query, " and ", " ")
	query = strings.ReplaceAll(query, " or ", " ")
	query = strings.ReplaceAll(query, " not ", " -")
	query = strings.ReplaceAll(query, "+", "")

	return query
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
