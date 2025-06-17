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

type AdvancedSearchEngine struct {
	index      *InvertedIndex
	stopWords  map[string]bool
	pageRank   map[string]float64
	linkGraph  map[string][]string
	synonyms   map[string][]string
	queryCache map[string][]SearchResult
	idfScores  map[string]float64
	totalDocs  int
}

func NewAdvancedSearchEngine() *AdvancedSearchEngine {
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

	engine := &AdvancedSearchEngine{
		index:      LoadIndex(),
		stopWords:  stopWords,
		pageRank:   make(map[string]float64),
		linkGraph:  make(map[string][]string),
		synonyms:   synonyms,
		queryCache: make(map[string][]SearchResult),
		idfScores:  make(map[string]float64),
		totalDocs:  0,
	}

	if engine.index != nil {
		engine.totalDocs = len(engine.index.Docs)
	}

	return engine
}

func (ase *AdvancedSearchEngine) Search(query string, maxResults int) []SearchResult {
	log.Printf("Starting advanced search for query: '%s'", query)

	if ase.index == nil || len(ase.index.Terms) == 0 {
		log.Printf("Index is nil or empty")
		return nil
	}

	cacheKey := strings.ToLower(query)
	if cached, exists := ase.queryCache[cacheKey]; exists {
		log.Printf("Returning cached results for query: '%s'", query)
		return ase.limitResults(cached, maxResults)
	}

	queryTerms := ase.processAdvancedQuery(query)
	log.Printf("Processed query terms: %v", queryTerms)

	candidates := ase.findAdvancedCandidates(queryTerms)
	log.Printf("Found %d candidates", len(candidates))

	if len(candidates) == 0 {
		return nil
	}

	results := ase.scoreAdvancedResults(queryTerms, candidates, query)
	log.Printf("Scored %d documents", len(results))

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	for i := range results {
		results[i].Rank = i + 1
	}

	ase.queryCache[cacheKey] = results
	log.Printf("Returning %d results", len(results))
	return ase.limitResults(results, maxResults)
}

func (ase *AdvancedSearchEngine) processAdvancedQuery(query string) []string {
	query = strings.ToLower(query)

	phrases := ase.extractPhrases(query)
	terms := ase.tokenize(query)

	var allTerms []string

	for _, phrase := range phrases {
		allTerms = append(allTerms, phrase)
	}

	for _, term := range terms {
		if !ase.stopWords[term] && len(term) > 1 {
			allTerms = append(allTerms, term)

			if synonyms, exists := ase.synonyms[term]; exists {
				for _, synonym := range synonyms {
					allTerms = append(allTerms, synonym)
				}
			}

			stemmed := ase.stemWord(term)
			if stemmed != term && len(stemmed) > 2 {
				allTerms = append(allTerms, stemmed)
			}
		}
	}

	return ase.removeDuplicates(allTerms)
}

func (ase *AdvancedSearchEngine) extractPhrases(query string) []string {
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

func (ase *AdvancedSearchEngine) tokenize(text string) []string {
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

func (ase *AdvancedSearchEngine) stemWord(word string) string {
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

func (ase *AdvancedSearchEngine) findAdvancedCandidates(queryTerms []string) map[string]bool {
	candidates := make(map[string]bool)
	termScores := make(map[string]float64)

	for _, term := range queryTerms {
		if docList, exists := ase.index.Terms[term]; exists {
			weight := ase.calculateTermWeight(term)

			for _, termFreq := range docList {
				candidates[termFreq.URL] = true
				termScores[termFreq.URL] += weight * termFreq.Score
			}
		}
	}

	if len(candidates) > 100 {
		threshold := ase.calculateThreshold(termScores)

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

func (ase *AdvancedSearchEngine) calculateTermWeight(term string) float64 {
	if idf, exists := ase.idfScores[term]; exists {
		return idf
	}

	docFreq := len(ase.index.Terms[term])
	if docFreq == 0 {
		return 0
	}

	totalDocs := float64(ase.totalDocs)
	if totalDocs == 0 {
		totalDocs = float64(len(ase.index.Docs))
	}

	idf := math.Log(totalDocs/float64(docFreq)) + 1
	ase.idfScores[term] = idf
	return idf
}

func (ase *AdvancedSearchEngine) calculateThreshold(termScores map[string]float64) float64 {
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

func (ase *AdvancedSearchEngine) scoreAdvancedResults(queryTerms []string, candidates map[string]bool, originalQuery string) []SearchResult {
	var results []SearchResult

	for url := range candidates {
		doc, exists := ase.index.Docs[url]
		if !exists {
			continue
		}

		titleScore := ase.calculateTextScore(queryTerms, doc.Title, 4.0)
		urlScore := ase.calculateURLScore(queryTerms, doc.URL)
		contentScore := ase.calculateTextScore(queryTerms, doc.Content, 1.0)

		proximityScore := ase.calculateProximityScore(queryTerms, doc.Content)
		phraseScore := ase.calculatePhraseScore(originalQuery, doc.Content, doc.Title)

		pageRank := ase.getPageRank(doc.URL)
		freshnessScore := 0.5

		totalScore := (titleScore*0.35 + urlScore*0.1 + contentScore*0.3 +
			proximityScore*0.1 + phraseScore*0.1 +
			pageRank*0.03 + freshnessScore*0.02)

		if totalScore > 0 {
			snippet := ase.generateAdvancedSnippet(doc.Content, queryTerms, 200)

			result := SearchResult{
				URL:            doc.URL,
				Title:          doc.Title,
				Snippet:        snippet,
				Score:          totalScore,
				TitleScore:     titleScore,
				URLScore:       urlScore,
				ContentScore:   contentScore,
				PageRank:       pageRank,
				FreshnessScore: freshnessScore,
			}

			results = append(results, result)
		}
	}

	return results
}

func (ase *AdvancedSearchEngine) calculateTextScore(queryTerms []string, text string, weight float64) float64 {
	if text == "" {
		return 0
	}

	textLower := strings.ToLower(text)
	words := ase.tokenize(textLower)
	wordCount := make(map[string]int)

	for _, word := range words {
		wordCount[word]++
	}

	score := 0.0
	totalWords := float64(len(words))

	for _, term := range queryTerms {
		if count, exists := wordCount[term]; exists {
			tf := float64(count) / totalWords
			idf := ase.calculateTermWeight(term)
			tfidf := tf * idf

			positionBoost := ase.calculatePositionBoost(term, textLower)
			score += tfidf * positionBoost
		}
	}

	return score * weight
}

func (ase *AdvancedSearchEngine) calculatePositionBoost(term, text string) float64 {
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

func (ase *AdvancedSearchEngine) calculateURLScore(queryTerms []string, url string) float64 {
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

func (ase *AdvancedSearchEngine) calculateProximityScore(queryTerms []string, content string) float64 {
	if len(queryTerms) < 2 {
		return 0
	}

	contentLower := strings.ToLower(content)
	words := ase.tokenize(contentLower)

	maxProximity := 0.0

	for i := 0; i < len(queryTerms); i++ {
		for j := i + 1; j < len(queryTerms); j++ {
			term1, term2 := queryTerms[i], queryTerms[j]

			pos1 := ase.findWordPositions(words, term1)
			pos2 := ase.findWordPositions(words, term2)

			minDistance := ase.findMinDistance(pos1, pos2)
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

func (ase *AdvancedSearchEngine) findWordPositions(words []string, term string) []int {
	var positions []int
	for i, word := range words {
		if word == term {
			positions = append(positions, i)
		}
	}
	return positions
}

func (ase *AdvancedSearchEngine) findMinDistance(pos1, pos2 []int) int {
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

func (ase *AdvancedSearchEngine) calculatePhraseScore(originalQuery, content, title string) float64 {
	phrases := ase.extractPhrases(originalQuery)
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

func (ase *AdvancedSearchEngine) getPageRank(url string) float64 {
	if rank, exists := ase.pageRank[url]; exists {
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

func (ase *AdvancedSearchEngine) calculateFreshnessScore(timestamp string) float64 {
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

func (ase *AdvancedSearchEngine) generateAdvancedSnippet(content string, queryTerms []string, maxLength int) string {
	if content == "" {
		return "No content available"
	}

	sentences := ase.splitIntoSentences(content)
	bestSentences := ase.findBestSentences(sentences, queryTerms, 2)

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

	snippet = ase.highlightTerms(snippet, queryTerms)

	return snippet
}

func (ase *AdvancedSearchEngine) splitIntoSentences(text string) []string {
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

func (ase *AdvancedSearchEngine) findBestSentences(sentences []string, queryTerms []string, maxSentences int) []string {
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

func (ase *AdvancedSearchEngine) highlightTerms(text string, queryTerms []string) string {
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

func (ase *AdvancedSearchEngine) removeDuplicates(slice []string) []string {
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

func (ase *AdvancedSearchEngine) limitResults(results []SearchResult, maxResults int) []SearchResult {
	if len(results) <= maxResults {
		return results
	}
	return results[:maxResults]
}
