package main

import (
	"bufio"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type TermFreq struct {
	URL   string
	Score float64
}

type InvertedIndex struct {
	Terms *sync.Map
	Docs  *sync.Map
}

type Document struct {
	URL      string
	Title    string
	Content  string
	Length   int
	PageRank float64
}

type Page struct {
	URL     string
	Title   string
	Content string
	Links   []string
	Hash    string
	Crawled time.Time
}

type Indexer struct {
	index     *InvertedIndex
	stopWords map[string]struct{}
}

func CreateIndexer() *Indexer {
	stopWords := loadIndexerStopWords()

	return &Indexer{
		index: &InvertedIndex{
			Terms: &sync.Map{},
			Docs:  &sync.Map{},
		},
		stopWords: stopWords,
	}
}

func (idx *Indexer) BuildIndex() *InvertedIndex {
	pages := idx.loadPages()
	if len(pages) == 0 {
		return idx.index
	}

	pageRanks := idx.calculatePageRank(pages)

	var wg sync.WaitGroup
	sem := make(chan struct{}, 100)

	for _, page := range pages {
		wg.Add(1)
		go func(p Page) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			doc := Document{
				URL:      p.URL,
				Title:    p.Title,
				Content:  p.Content,
				Length:   len(strings.Fields(p.Content)),
				PageRank: pageRanks[p.URL],
			}

			idx.index.Docs.Store(p.URL, doc)
			idx.processDocument(doc)
		}(page)
	}

	wg.Wait()
	idx.saveIndex()
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
	allURLs := make(map[string]struct{})

	for _, page := range pages {
		allURLs[page.URL] = struct{}{}
		linkGraph[page.URL] = page.Links

		for _, link := range page.Links {
			if _, exists := allURLs[link]; exists {
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
		if _, stopWord := idx.stopWords[word]; !stopWord && len(word) > 1 {
			termFreqs[word]++

			stemmed := idx.stemWord(word)
			if stemmed != word && len(stemmed) > 1 {
				termFreqs[stemmed]++
			}
		}
	}

	for term, freq := range termFreqs {
		tf := float64(freq) / float64(len(words))
		score := tf * (1.0 + doc.PageRank)

		if strings.Contains(strings.ToLower(doc.Title), term) {
			score *= 2.0
		}

		termFreq := TermFreq{
			URL:   doc.URL,
			Score: score,
		}

		if existing, exists := idx.index.Terms.Load(term); exists {
			termFreqs := existing.([]TermFreq)
			termFreqs = append(termFreqs, termFreq)
			idx.index.Terms.Store(term, termFreqs)
		} else {
			idx.index.Terms.Store(term, []TermFreq{termFreq})
		}
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

	idx.index.Terms.Range(func(key, value interface{}) bool {
		term := key.(string)
		termFreqs := value.([]TermFreq)

		sort.Slice(termFreqs, func(i, j int) bool {
			return termFreqs[i].Score > termFreqs[j].Score
		})

		for _, tf := range termFreqs {
			line := term + "|" + tf.URL + "|" + strconv.FormatFloat(tf.Score, 'f', 6, 64) + "\n"
			termsFile.WriteString(line)
		}
		return true
	})

	docsFile, err := os.Create("data/docs.dat")
	if err != nil {
		return
	}
	defer docsFile.Close()

	idx.index.Docs.Range(func(key, value interface{}) bool {
		url := key.(string)
		doc := value.(Document)

		line := url + "|" + doc.Title + "|" +
			strings.ReplaceAll(doc.Content, "\n", " ") + "|" +
			strconv.Itoa(doc.Length) + "|" +
			strconv.FormatFloat(doc.PageRank, 'f', 6, 64) + "\n"
		docsFile.WriteString(line)
		return true
	})
}

func (idx *Indexer) stemWord(word string) string {
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

func LoadIndex() *InvertedIndex {
	index := &InvertedIndex{
		Terms: &sync.Map{},
		Docs:  &sync.Map{},
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

			termFreq := TermFreq{
				URL:   url,
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

			index.Docs.Store(url, Document{
				URL:      url,
				Title:    title,
				Content:  content,
				Length:   length,
				PageRank: pageRank,
			})
		}
	}

	return index
}

func loadIndexerStopWords() map[string]struct{} {
	stopWords := make(map[string]struct{})

	file, err := os.Open("data/stopwords.txt")
	if err != nil {
		return map[string]struct{}{
			"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {},
			"in": {}, "on": {}, "at": {}, "to": {}, "for": {}, "of": {},
			"with": {}, "by": {},
		}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if word != "" {
			stopWords[word] = struct{}{}
		}
	}

	return stopWords
}

func (idx *Indexer) AddDocument(doc Document) {
	idx.index.Docs.Store(doc.URL, doc)

	tokens := idx.tokenize(doc.Title + " " + doc.Content)

	for _, token := range tokens {
		if _, isStop := idx.stopWords[token]; !isStop && len(token) > 2 {
			var termFreqs []TermFreq
			if existing, ok := idx.index.Terms.Load(token); ok {
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

			idx.index.Terms.Store(token, termFreqs)
		}
	}
}

func (idx *Indexer) GetIndex() *InvertedIndex {
	return idx.index
}
