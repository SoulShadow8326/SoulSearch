package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
)

type Synset struct {
	Words     []string      `json:"words"`
	Relations [][2]string   `json:"relations"`
}

type PageData struct{
	URL  string
	Title string
	Content string
	LinkList []string
}

type SearchResult struct {
	URL	string
	Title	 string
	Snippet	string
	Score	float64
}

type IndexedDocument struct{
	URL string
	Title string
	Content string
	Tokens []string
}

var (
	lemmas map[string][]string
	synsets map[string]Synset
)

func LoadAllDocuments() ([]IndexedDocument, error){
	rows, err := DB.Query("Select url, title, content FROM pages")
	if err != nil{
		return nil ,err
	}
	defer rows.Close()

	var docs []IndexedDocument
	for rows.Next() {
		var url, title, content string
		if err := rows.Scan(&url, &title, &content); err != nil{
			continue
		}
		tokens := tokenize(content)
		docs = append(docs, IndexedDocument{
			URL: url,
			Title: title,
			Content: content,
			Tokens: tokens,
		})
	}
	return docs, nil
}

func LoadSynsetData(){
	f1, err := os.Open("data/lemmas.json")
	if err != nil{
		log.Fatal("Could not open lemmas.json:", err)
	}
	defer f1.Close()
	if err := json.NewDecoder(f1).Decode(&lemmas); err != nil{
		log.Fatal("Could not decode lemmas.json", err)
	}
	f2, err := os.Open("data/synsets.json")
	if err != nil{
		log.Fatal("Could not open synsets.json", err)
	}
	defer f2.Close()
	if err := json.NewDecoder(f2).Decode(&synsets); err != nil{
		log.Fatal("COuld not Decode synsets.json:", err)
	}
}

func GetSynonyms(word string) []string {
	seen := map[string]bool{}
	word = strings.ToLower(word)

	ids, ok := lemmas[word]
	if !ok {
		return nil
	}

	for _, id := range ids {
		syn, ok := synsets[id]
		if !ok {
			continue
		}
		for _, w := range syn.Words {
			if w != word {
				seen[w] = true
			}
		}
	}

	var out []string
	for w := range seen {
		out = append(out, w)
	}
	return out
}
