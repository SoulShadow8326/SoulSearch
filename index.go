package main

import (
	"encoding/json"
	"net"
	"sync"
)

type SharedIndex struct {
	index    *InvertedIndex
	mu       sync.RWMutex
	sockPath string
}

func CreateSharedIndex(sockPath string) *SharedIndex {
	return &SharedIndex{
		index: &InvertedIndex{
			Terms: &sync.Map{},
			Docs:  &sync.Map{},
		},
		sockPath: sockPath,
	}
}

func (si *SharedIndex) GetIndex() *InvertedIndex {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.index
}

func (si *SharedIndex) AddDocument(doc Document) {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.index.Docs.Store(doc.URL, doc)

	tokens := tokenizeText(doc.Title + " " + doc.Content)

	for _, token := range tokens {
		if len(token) > 2 {
			var termFreqs []TermFreq
			if existing, ok := si.index.Terms.Load(token); ok {
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

			si.index.Terms.Store(token, termFreqs)
		}
	}
}

func (si *SharedIndex) RequestLiveCrawl(query string) {
	conn, err := net.Dial("unix", si.sockPath)
	if err != nil {
		return
	}
	defer conn.Close()

	urls := generateCrawlURLs(query)
	msg := map[string]interface{}{
		"type":    "BULK_ADD",
		"payload": urls,
	}

	data, _ := json.Marshal(msg)
	conn.Write(append(data, '\n'))
}

func generateCrawlURLs(query string) []string {
	return []string{
		"https://en.wikipedia.org/wiki/Special:Search?search=" + query,
		"https://stackoverflow.com/search?q=" + query,
		"https://github.com/search?q=" + query,
		"https://duckduckgo.com/?q=" + query,
	}
}

func tokenizeText(text string) []string {
	var tokens []string
	var current string

	for _, char := range text {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			current += string(char)
		} else {
			if len(current) > 0 {
				tokens = append(tokens, current)
				current = ""
			}
		}
	}

	if len(current) > 0 {
		tokens = append(tokens, current)
	}

	return tokens
}

var globalSharedIndex *SharedIndex

func GetGlobalSharedIndex() *SharedIndex {
	if globalSharedIndex == nil {
		globalSharedIndex = CreateSharedIndex("/tmp/soulsearch.sock")
	}
	return globalSharedIndex
}
