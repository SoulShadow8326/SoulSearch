package main

import (
	"log"
	"math"
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
	index      *InvertedIndex
	stopWords  map[string]bool
	pageRank   map[string]float64
	linkGraph  map[string][]string
	synonyms   map[string][]string
	queryCache map[string][]SearchResult
	idfScores  map[string]float64
	totalDocs  int
}

func NewSearchEngine() *SearchEngine {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
	}

	synonyms := map[string][]string{
		"duck":     {"waterfowl", "bird", "aquatic", "pond", "swim", "quack", "mallard", "drake"},
		"car":      {"vehicle", "automobile", "auto", "motor"},
		"dog":      {"canine", "puppy", "hound", "pet"},
		"cat":      {"feline", "kitten", "kitty", "pet"},
		"house":    {"home", "residence", "dwelling", "building"},
		"big":      {"large", "huge", "enormous", "massive", "giant"},
		"small":    {"little", "tiny", "mini", "miniature", "petite"},
		"good":     {"great", "excellent", "amazing", "wonderful", "fantastic"},
		"bad":      {"terrible", "awful", "horrible", "poor", "dreadful"},
		"fast":     {"quick", "rapid", "speedy", "swift", "hasty"},
		"money":    {"cash", "currency", "funds", "capital", "wealth"},
		"work":     {"job", "employment", "career", "occupation", "labor"},
		"food":     {"meal", "cuisine", "dish", "nutrition", "nourishment"},
		"computer": {"pc", "laptop", "machine", "device", "technology"},
		"internet": {"web", "online", "net", "cyberspace"},
		"animal":   {"creature", "beast", "wildlife", "fauna"},
		"bird":     {"avian", "fowl", "winged", "feathered"},
		"water":    {"liquid", "aquatic", "fluid", "wet"},
		"swim":     {"float", "paddle", "aquatic", "water"},
	}

	engine := &SearchEngine{
		stopWords:  stopWords,
		pageRank:   make(map[string]float64),
		linkGraph:  make(map[string][]string),
		synonyms:   synonyms,
		queryCache: make(map[string][]SearchResult),
		idfScores:  make(map[string]float64),
		totalDocs:  0,
	}

	return engine
}

func (se *SearchEngine) LoadIndex(index *InvertedIndex) {
	se.index = index
	se.totalDocs = len(index.Docs)
	se.calculateIDF()
}

func (se *SearchEngine) calculateIDF() {
	se.idfScores = make(map[string]float64)
	for term, termFreqs := range se.index.Terms {
		docCount := len(termFreqs)
		if docCount > 0 {
			se.idfScores[term] = math.Log(float64(se.totalDocs) / float64(docCount))
		}
	}
}

func (se *SearchEngine) Search(query string, limit int) ([]SearchResult, string) {
	start := time.Now()

	results := se.SearchAdvanced(query, limit)

	elapsed := time.Since(start)
	return results, elapsed.String()
}

func (se *SearchEngine) SearchAdvanced(query string, maxResults int) []SearchResult {
	log.Printf("Starting advanced search for query: '%s'", query)

	if se.index == nil || len(se.index.Terms) == 0 {
		log.Printf("Index is nil or empty")
		return nil
	}

	cacheKey := strings.ToLower(query)
	if cached, exists := se.queryCache[cacheKey]; exists {
		log.Printf("Returning cached results for query: '%s'", query)
		return se.limitResults(cached, maxResults)
	}

	queryTerms := se.processAdvancedQuery(query)
	log.Printf("Processed query terms: %v", queryTerms)

	candidates := se.findAdvancedCandidates(queryTerms)
	log.Printf("Found %d candidates", len(candidates))

	if len(candidates) == 0 {
		return nil
	}

	results := se.scoreAdvancedResults(queryTerms, candidates, query)
	log.Printf("Scored %d documents", len(results))

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	for i := range results {
		results[i].Rank = i + 1
	}

	se.queryCache[cacheKey] = results
	log.Printf("Returning %d results", len(results))
	return se.limitResults(results, maxResults)
}

func (se *SearchEngine) processAdvancedQuery(query string) []string {
	query = strings.ToLower(query)

	phrases := se.extractPhrases(query)
	terms := se.tokenize(query)

	var allTerms []string

	allTerms = append(allTerms, phrases...)

	for _, term := range terms {
		if !se.stopWords[term] && len(term) > 1 {
			allTerms = append(allTerms, term)

			if synonyms, exists := se.synonyms[term]; exists {
				allTerms = append(allTerms, synonyms...)
			}

			stemmed := se.stemWord(term)
			if stemmed != term && len(stemmed) > 2 {
				allTerms = append(allTerms, stemmed)
			}
		}
	}

	return se.removeDuplicates(allTerms)
}

func (se *SearchEngine) extractPhrases(query string) []string {
	var phrases []string

	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindAllStringSubmatch(query, -1)

	for _, match := range matches {
		if len(match) > 1 {
			phrases = append(phrases, strings.ToLower(match[1]))
		}
	}

	return phrases
}

func (se *SearchEngine) tokenize(text string) []string {
	var words []string
	var currentWord strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			currentWord.WriteRune(unicode.ToLower(r))
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
	if len(word) <= 3 {
		return word
	}

	suffixes := []string{"ing", "ed", "er", "est", "ly", "tion", "sion", "ness", "ment", "able", "ible", "ous", "ful", "less", "ish", "ive", "al", "ic", "ical", "ate", "ize", "ise"}

	for _, suffix := range suffixes {
		if strings.HasSuffix(word, suffix) && len(word) > len(suffix)+2 {
			return word[:len(word)-len(suffix)]
		}
	}

	if strings.HasSuffix(word, "s") && len(word) > 3 && !strings.HasSuffix(word, "ss") {
		return word[:len(word)-1]
	}

	return word
}

func (se *SearchEngine) findAdvancedCandidates(queryTerms []string) map[string]bool {
	candidates := make(map[string]bool)
	termScores := make(map[string]float64)

	for _, term := range queryTerms {
		if docList, exists := se.index.Terms[term]; exists {
			weight := se.calculateTermWeight(term)

			for _, termFreq := range docList {
				candidates[termFreq.URL] = true
				termScores[termFreq.URL] += weight * termFreq.Score
			}
		}
	}

	if len(candidates) > 100 {
		threshold := se.calculateThreshold(termScores)

		filteredCandidates := make(map[string]bool)
		for url := range candidates {
			if termScores[url] >= threshold {
				filteredCandidates[url] = true
			}
		}
		return filteredCandidates
	}

	return candidates
}

func (se *SearchEngine) calculateTermWeight(term string) float64 {
	if idf, exists := se.idfScores[term]; exists {
		return idf
	}

	docFreq := len(se.index.Terms[term])
	if docFreq == 0 {
		return 0
	}

	totalDocs := float64(se.totalDocs)
	if totalDocs == 0 {
		totalDocs = float64(len(se.index.Docs))
	}

	idf := math.Log(totalDocs/float64(docFreq)) + 1
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

	percentile := int(float64(len(scores)) * 0.6)
	return scores[percentile]
}

func (se *SearchEngine) scoreAdvancedResults(queryTerms []string, candidates map[string]bool, originalQuery string) []SearchResult {
	var results []SearchResult

	for url := range candidates {
		doc, exists := se.index.Docs[url]
		if !exists {
			continue
		}

		titleScore := se.calculateTextScore(queryTerms, doc.Title, 4.0)
		urlScore := se.calculateURLScore(queryTerms, doc.URL)
		contentScore := se.calculateTextScore(queryTerms, doc.Content, 1.0)

		proximityScore := se.calculateProximityScore(queryTerms, doc.Content)
		phraseScore := se.calculatePhraseScore(originalQuery, doc.Content, doc.Title)

		pageRank := se.getPageRank(doc.URL)
		freshnessScore := se.calculateFreshnessScore("")

		totalScore := (titleScore*0.35 + urlScore*0.1 + contentScore*0.3 +
			proximityScore*0.1 + phraseScore*0.1 +
			pageRank*0.03 + freshnessScore*0.02)

		if totalScore > 0 {
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

	for _, term := range queryTerms {
		if count, exists := wordCount[term]; exists {
			tf := float64(count) / totalWords
			idf := se.calculateTermWeight(term)
			tfidf := tf * idf

			positionBoost := se.calculatePositionBoost(term, textLower)
			score += tfidf * positionBoost
		}
	}

	return score * weight
}

func (se *SearchEngine) calculatePositionBoost(term, text string) float64 {
	index := strings.Index(text, term)
	if index == -1 {
		return 1.0
	}

	textLen := float64(len(text))
	position := float64(index) / textLen

	if position < 0.1 {
		return 3.0
	} else if position < 0.3 {
		return 2.0
	} else if position < 0.6 {
		return 1.5
	}

	return 1.0
}

func (se *SearchEngine) calculateURLScore(queryTerms []string, url string) float64 {
	urlLower := strings.ToLower(url)
	score := 0.0

	for _, term := range queryTerms {
		if strings.Contains(urlLower, term) {
			score += 2.0
		}
	}

	if strings.Contains(urlLower, "https") {
		score += 0.1
	}

	depth := strings.Count(urlLower, "/") - 2
	if depth < 3 {
		score += 0.3
	}

	return score
}

func (se *SearchEngine) calculateProximityScore(queryTerms []string, content string) float64 {
	if len(queryTerms) < 2 {
		return 0
	}

	contentLower := strings.ToLower(content)
	words := se.tokenize(contentLower)

	maxProximity := 0.0

	for i := 0; i < len(queryTerms); i++ {
		for j := i + 1; j < len(queryTerms); j++ {
			term1, term2 := queryTerms[i], queryTerms[j]

			pos1 := se.findWordPositions(words, term1)
			pos2 := se.findWordPositions(words, term2)

			minDistance := se.findMinDistance(pos1, pos2)
			if minDistance > 0 && minDistance < 15 {
				proximity := 2.0 / float64(minDistance)
				if proximity > maxProximity {
					maxProximity = proximity
				}
			}
		}
	}

	return maxProximity
}

func (se *SearchEngine) findWordPositions(words []string, term string) []int {
	var positions []int
	for i, word := range words {
		if word == term {
			positions = append(positions, i)
		}
	}
	return positions
}

func (se *SearchEngine) findMinDistance(pos1, pos2 []int) int {
	if len(pos1) == 0 || len(pos2) == 0 {
		return -1
	}

	minDist := math.MaxInt32

	for _, p1 := range pos1 {
		for _, p2 := range pos2 {
			dist := int(math.Abs(float64(p1 - p2)))
			if dist < minDist {
				minDist = dist
			}
		}
	}

	return minDist
}

func (se *SearchEngine) calculatePhraseScore(originalQuery, content, title string) float64 {
	phrases := se.extractPhrases(originalQuery)
	if len(phrases) == 0 {
		return 0
	}

	score := 0.0
	contentLower := strings.ToLower(content)
	titleLower := strings.ToLower(title)

	for _, phrase := range phrases {
		if strings.Contains(titleLower, phrase) {
			score += 3.0
		}
		if strings.Contains(contentLower, phrase) {
			score += 1.5
		}
	}

	return score
}

func (se *SearchEngine) getPageRank(url string) float64 {
	if rank, exists := se.pageRank[url]; exists {
		return rank
	}

	if strings.Contains(url, "wikipedia.org") {
		return 0.95
	} else if strings.Contains(url, ".edu") {
		return 0.9
	} else if strings.Contains(url, ".gov") {
		return 0.85
	} else if strings.Contains(url, "stackoverflow.com") {
		return 0.8
	} else if strings.Contains(url, "github.com") {
		return 0.75
	}

	return 0.4
}

func (se *SearchEngine) calculateFreshnessScore(timestamp string) float64 {
	if timestamp == "" {
		return 0.4
	}

	timeValue, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return 0.4
	}

	age := time.Since(timeValue).Hours() / 24

	if age < 1 {
		return 1.0
	} else if age < 7 {
		return 0.9
	} else if age < 30 {
		return 0.7
	} else if age < 365 {
		return 0.5
	}

	return 0.3
}

func (se *SearchEngine) generateAdvancedSnippet(content string, queryTerms []string, maxLength int) string {
	if content == "" {
		return "No content available"
	}

	sentences := se.splitIntoSentences(content)
	bestSentences := se.findBestSentences(sentences, queryTerms, 2)

	snippet := strings.Join(bestSentences, " ")

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
