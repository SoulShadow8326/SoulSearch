package main

import (
	"log"
	"math"
	"sort"
	"strings"
	"unicode"
)

type SearchResult struct {
	URL     string
	Title   string
	Snippet string
	Score   float64
	Rank    int
}

type SearchEngine struct {
	index     *InvertedIndex
	stopWords map[string]bool
}

func NewSearchEngine() *SearchEngine {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "can": true, "this": true, "that": true,
		"these": true, "those": true, "i": true, "you": true, "he": true, "she": true,
		"it": true, "we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true, "its": true,
		"our": true, "their": true,
	}

	return &SearchEngine{
		index:     LoadIndex(),
		stopWords: stopWords,
	}
}

func (se *SearchEngine) Search(query string, maxResults int) []SearchResult {
	log.Printf("Starting search for query: '%s'", query)

	if se.index == nil || len(se.index.Terms) == 0 {
		log.Printf("Index is nil or empty")
		return nil
	}

	queryTerms := se.processQuery(query)
	log.Printf("Processed query terms: %v", queryTerms)

	if len(queryTerms) == 0 {
		log.Printf("No valid query terms")
		return nil
	}

	candidates := se.findCandidates(queryTerms)
	log.Printf("Found %d candidates", len(candidates))

	scored := se.scoreDocuments(candidates, queryTerms, query)
	log.Printf("Scored %d documents", len(scored))

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	for i := range scored {
		scored[i].Rank = i + 1
		scored[i].Snippet = se.generateSnippet(scored[i].URL, queryTerms)
	}

	log.Printf("Returning %d results", len(scored))
	return scored
}

func (se *SearchEngine) processQuery(query string) []string {
	words := se.tokenize(query)
	var terms []string

	for _, word := range words {
		if !se.stopWords[word] && len(word) > 1 {
			terms = append(terms, word)
		}
	}

	return terms
}

func (se *SearchEngine) tokenize(text string) []string {
	text = strings.ToLower(text)

	var words []string
	var word strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			word.WriteRune(r)
		} else {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
		}
	}

	if word.Len() > 0 {
		words = append(words, word.String())
	}

	return words
}

func (se *SearchEngine) findCandidates(queryTerms []string) map[string]bool {
	candidates := make(map[string]bool)

	for _, term := range queryTerms {
		if termFreqs, exists := se.index.Terms[term]; exists {
			for _, tf := range termFreqs {
				candidates[tf.URL] = true
			}
		}
	}

	return candidates
}

func (se *SearchEngine) scoreDocuments(candidates map[string]bool, queryTerms []string, originalQuery string) []SearchResult {
	var results []SearchResult

	for url := range candidates {
		doc, exists := se.index.Docs[url]
		if !exists {
			continue
		}

		score := se.calculateScore(doc, queryTerms, originalQuery)

		if score > 0 {
			results = append(results, SearchResult{
				URL:   url,
				Title: doc.Title,
				Score: score,
			})
		}
	}

	return results
}

func (se *SearchEngine) calculateScore(doc Document, queryTerms []string, originalQuery string) float64 {
	var totalScore float64
	termMatches := 0

	docText := strings.ToLower(doc.Title + " " + doc.Content)

	for _, term := range queryTerms {
		if termFreqs, exists := se.index.Terms[term]; exists {
			for _, tf := range termFreqs {
				if tf.URL == doc.URL {
					totalScore += tf.Score
					termMatches++
					break
				}
			}
		}
	}

	if termMatches == 0 {
		return 0
	}

	queryMatchRatio := float64(termMatches) / float64(len(queryTerms))
	totalScore *= queryMatchRatio

	phraseQuery := strings.ToLower(originalQuery)
	if strings.Contains(docText, phraseQuery) {
		totalScore *= 2.0
	}

	titleBoost := 1.0
	for _, term := range queryTerms {
		if strings.Contains(strings.ToLower(doc.Title), term) {
			titleBoost += 0.5
		}
	}
	totalScore *= titleBoost

	lengthPenalty := 1.0 / (1.0 + math.Log(float64(doc.Length)/1000.0+1))
	totalScore *= lengthPenalty

	totalScore *= (1.0 + doc.PageRank)

	return totalScore
}

func (se *SearchEngine) generateSnippet(url string, queryTerms []string) string {
	doc, exists := se.index.Docs[url]
	if !exists {
		return ""
	}

	content := doc.Content
	if len(content) == 0 {
		return doc.Title
	}

	words := strings.Fields(content)
	if len(words) <= 30 {
		return content
	}

	bestStart := 0
	bestScore := 0

	for i := 0; i < len(words)-30; i++ {
		snippet := strings.Join(words[i:i+30], " ")
		score := 0

		for _, term := range queryTerms {
			if strings.Contains(strings.ToLower(snippet), term) {
				score++
			}
		}

		if score > bestScore {
			bestScore = score
			bestStart = i
		}
	}

	snippet := strings.Join(words[bestStart:bestStart+30], " ")

	for _, term := range queryTerms {
		capitalized := strings.ToUpper(term[:1]) + term[1:]
		re := strings.NewReplacer(
			term, "<b>"+term+"</b>",
			capitalized, "<b>"+capitalized+"</b>",
			strings.ToUpper(term), "<b>"+strings.ToUpper(term)+"</b>",
		)
		snippet = re.Replace(snippet)
	}

	return snippet + "..."
}
