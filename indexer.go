package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type TermFreq struct {
	URL   string
	Score float64
}

type InvertedIndex struct {
	Terms map[string][]TermFreq
	Docs  map[string]Document
}

type Document struct {
	URL      string
	Title    string
	Content  string
	Length   int
	PageRank float64
}

type Indexer struct {
	index     *InvertedIndex
	stopWords map[string]bool
}

func NewIndexer() *Indexer {
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

	return &Indexer{
		index: &InvertedIndex{
			Terms: make(map[string][]TermFreq),
			Docs:  make(map[string]Document),
		},
		stopWords: stopWords,
	}
}

func (idx *Indexer) BuildIndex() *InvertedIndex {
	pages := idx.loadPages()
	if len(pages) == 0 {
		fmt.Println("No pages found. Run crawler first.")
		return idx.index
	}

	pageRanks := idx.calculatePageRank(pages)

	for _, page := range pages {
		doc := Document{
			URL:      page.URL,
			Title:    page.Title,
			Content:  page.Content,
			Length:   len(strings.Fields(page.Content)),
			PageRank: pageRanks[page.URL],
		}

		idx.index.Docs[page.URL] = doc
		idx.processDocument(doc)
	}

	idx.saveIndex()
	fmt.Printf("Indexed %d documents with %d unique terms\n",
		len(idx.index.Docs), len(idx.index.Terms))
	return idx.index
}

func (idx *Indexer) loadPages() []Page {
	file, err := os.Open("data/pages.dat")
	if err != nil {
		return nil
	}
	defer file.Close()

	var pages []Page
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) >= 4 {
			pages = append(pages, Page{
				URL:     parts[0],
				Title:   parts[1],
				Content: parts[2],
				Hash:    parts[3],
			})
		}
	}

	return pages
}

func (idx *Indexer) calculatePageRank(pages []Page) map[string]float64 {
	linkGraph := make(map[string][]string)
	inlinks := make(map[string]int)
	allURLs := make(map[string]bool)

	for _, page := range pages {
		allURLs[page.URL] = true
		linkGraph[page.URL] = page.Links

		for _, link := range page.Links {
			if allURLs[link] {
				inlinks[link]++
			}
		}
	}

	pageRanks := make(map[string]float64)
	for url := range allURLs {
		pageRanks[url] = 1.0
	}

	dampingFactor := 0.85
	for i := 0; i < 50; i++ {
		newRanks := make(map[string]float64)

		for url := range allURLs {
			newRanks[url] = (1.0 - dampingFactor) / float64(len(allURLs))

			for sourceURL, links := range linkGraph {
				for _, link := range links {
					if link == url && len(links) > 0 {
						newRanks[url] += dampingFactor * pageRanks[sourceURL] / float64(len(links))
					}
				}
			}
		}

		pageRanks = newRanks
	}

	return pageRanks
}

func (idx *Indexer) processDocument(doc Document) {
	words := idx.tokenize(doc.Title + " " + doc.Content)
	termFreqs := make(map[string]int)

	for _, word := range words {
		if !idx.stopWords[word] && len(word) > 2 {
			termFreqs[word]++
		}
	}

	for term, freq := range termFreqs {
		tf := float64(freq) / float64(len(words))
		score := tf * (1.0 + doc.PageRank)

		if strings.Contains(strings.ToLower(doc.Title), term) {
			score *= 2.0
		}

		idx.index.Terms[term] = append(idx.index.Terms[term], TermFreq{
			URL:   doc.URL,
			Score: score,
		})
	}
}

func (idx *Indexer) tokenize(text string) []string {
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

func (idx *Indexer) saveIndex() {
	os.MkdirAll("data", 0755)

	termsFile, err := os.Create("data/terms.dat")
	if err != nil {
		return
	}
	defer termsFile.Close()

	for term, termFreqs := range idx.index.Terms {
		sort.Slice(termFreqs, func(i, j int) bool {
			return termFreqs[i].Score > termFreqs[j].Score
		})

		for _, tf := range termFreqs {
			line := fmt.Sprintf("%s|%s|%.6f\n", term, tf.URL, tf.Score)
			termsFile.WriteString(line)
		}
	}

	docsFile, err := os.Create("data/docs.dat")
	if err != nil {
		return
	}
	defer docsFile.Close()

	for url, doc := range idx.index.Docs {
		line := fmt.Sprintf("%s|%s|%s|%d|%.6f\n",
			url, doc.Title,
			strings.ReplaceAll(doc.Content, "\n", " "),
			doc.Length, doc.PageRank)
		docsFile.WriteString(line)
	}
}

func LoadIndex() *InvertedIndex {
	index := &InvertedIndex{
		Terms: make(map[string][]TermFreq),
		Docs:  make(map[string]Document),
	}

	termsFile, err := os.Open("data/terms.dat")
	if err != nil {
		return index
	}
	defer termsFile.Close()

	scanner := bufio.NewScanner(termsFile)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) >= 3 {
			term := parts[0]
			url := parts[1]
			score, _ := strconv.ParseFloat(parts[2], 64)

			index.Terms[term] = append(index.Terms[term], TermFreq{
				URL:   url,
				Score: score,
			})
		}
	}

	docsFile, err := os.Open("data/docs.dat")
	if err != nil {
		return index
	}
	defer docsFile.Close()

	scanner = bufio.NewScanner(docsFile)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			url := parts[0]
			title := parts[1]
			content := parts[2]
			length, _ := strconv.Atoi(parts[3])
			pageRank, _ := strconv.ParseFloat(parts[4], 64)

			index.Docs[url] = Document{
				URL:      url,
				Title:    title,
				Content:  content,
				Length:   length,
				PageRank: pageRank,
			}
		}
	}

	return index
}
