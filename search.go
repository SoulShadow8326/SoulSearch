package main

import (
	"sort"
	"strings"
	"unicode"
)

func Search(query string) []SearchResult {
	rows, err := DB.Query("SELECT url, title, content FROM pages")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []SearchResult
	tokens := expandQuery(tokenize(query))

	for rows.Next() {
		var url, title, content string
		if err := rows.Scan(&url, &title, &content); err != nil {
			continue
		}

		score := computeScore(content, tokens)
		if score > 0 {
			results = append(results, SearchResult{
				URL:     url,
				Title:   title,
				Snippet: snippet(content, tokens),
				Score:   score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 20 {
		results = results[:20]
	}

	return results
}

func tokenize(s string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func computeScore(text string, tokens []string) float64 {
	contentTokens := tokenize(text)

	counts := make(map[string]int)
	for _, t := range contentTokens {
		counts[t]++
	}
	var score float64
	for _, token := range tokens {
		score += float64(counts[token])
	}
	return score
}

func snippet(text string, tokens []string) string {
	lowerText := strings.ToLower(text)
	for _, token := range tokens {
		idx := strings.Index(lowerText, token)
		if idx != -1 {
			start := idx - 60
			end := idx + 60

			if start < 0 {
				start = 0
			}
			if end > len(text) {
				end = len(text)
			}

			return "..." + strings.TrimSpace(text[start:end]) + "..."
		}
	}
	return text[:min(120, len(text))] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func expandQuery(tokens []string) []string {
	seen := map[string]bool{}
	var expanded []string

	for _, t := range tokens {
		if !seen[t] {
			seen[t] = true
			expanded = append(expanded, t)
		}
		for _, syn := range GetSynonyms(t) {
			if !seen[syn] {
				seen[syn] = true
				expanded = append(expanded, syn)
			}
		}
	}
	return expanded
}
