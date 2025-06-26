package main

import (
	"sort"
	"strings"
	"unicode"
	"log"
	"math"
)

func Search(query string) []SearchResult {
	tokens := expandQuery(tokenize(query))
	if len(tokens) == 0 {
		return nil
	}

	pageTermFreq := map[int]map[string]int{}
	docFreq := map[string]int{}
	totalDocs := 0

	err := DB.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&totalDocs)
	if err != nil {
		log.Println("Failed to count total docs:", err)
		return nil
	}

	for _, token := range tokens {
		rows, err := DB.Query(`SELECT page_id, frequency FROM inverted_index WHERE term = ?`, token)
		if err != nil {
			continue
		}
		defer rows.Close()

		dfCounted := map[int]bool{}
		for rows.Next() {
			var pageID, freq int
			if err := rows.Scan(&pageID, &freq); err == nil {
				if _, ok := pageTermFreq[pageID]; !ok {
					pageTermFreq[pageID] = map[string]int{}
				}
				pageTermFreq[pageID][token] = freq

				if !dfCounted[pageID] {
					docFreq[token]++
					dfCounted[pageID] = true
				}
			}
		}
	}

	var results []SearchResult
	for pageID, termFreqs := range pageTermFreq {
		var tfidfScore float64
		for term, freq := range termFreqs {
			tf := float64(freq)
			idf := 1.0
			if docFreq[term] > 0 {
				idf = math.Log(float64(totalDocs) / float64(docFreq[term]))
			}
			tfidfScore += tf * idf
		}

		var url, title, content string
		var pagerank float64
		err := DB.QueryRow(`SELECT url, title, content, pagerank FROM pages WHERE id = ?`, pageID).
			Scan(&url, &title, &content, &pagerank)
		if err != nil {
			continue
		}

		finalScore := tfidfScore*0.7 + pagerank*0.3
		results = append(results, SearchResult{
			URL:     url,
			Title:   title,
			Snippet: snippet(content, tokens),
			Score:   finalScore,
		})
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

func ComputePageRank(iterations int, damping float64) {
	links := make(map[string][]string)
	pages := make(map[string]float64)

	rows, err := DB.Query("SELECT url FROM pages")
	if err != nil {
		log.Println("Failed to load pages:", err)
		return
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err == nil {
			pages[url] = 1.0
			urls = append(urls, url)
			log.Println(urls)
		}
	}

	linkRows, err := DB.Query("SELECT from_url, to_url FROM links")
	if err != nil {
		log.Println("Failed to load links:", err)
		return
	}
	defer linkRows.Close()

	for linkRows.Next() {
		var from, to string
		if err := linkRows.Scan(&from, &to); err == nil {
			links[from] = append(links[from], to)
		}
	}

	N := float64(len(pages))
	for i := 0; i < iterations; i++ {
		newScores := make(map[string]float64)
		for page := range pages {
			newScores[page] = (1.0 - damping) / N
		}

		for page, outLinks := range links {
			if len(outLinks) == 0 {
				continue
			}
			share := damping * pages[page] / float64(len(outLinks))
			for _, out := range outLinks {
				newScores[out] += share
			}
		}
		pages = newScores
	}

	tx, err := DB.Begin()
	if err != nil {
		log.Println("Failed to begin PageRank tx:", err)
		return
	}
	stmt, err := tx.Prepare("UPDATE pages SET pagerank = ? WHERE url = ?")
	if err != nil {
		log.Println("Failed to prepare pagerank update:", err)
		return
	}
	defer stmt.Close()

	for url, score := range pages {
		_, _ = stmt.Exec(score, url)
	}
	tx.Commit()
}
