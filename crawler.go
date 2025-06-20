package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Page struct {
	URL         string
	Title       string
	Content     string
	Links       []string
	Hash        string
	Crawled     time.Time
	Depth       int
	Quality     float64
	Size        int
	Domain      string
	Language    string
	Redirects   int
	ContentType ContentType
	Keywords    []string
	Entities    []string
	ReadingTime int
	ExtraMeta   map[string]string
}

type SeedURL struct {
	URL         string
	Priority    float64
	Topic       string
	Source      string
	Reliability float64
}

type CrawledDocument struct {
	URL         string
	Title       string
	Content     string
	Description string
	Keywords    []string
	Links       []string
	Language    string
	Quality     float64
	CrawledAt   time.Time
	Size        int
	Domain      string
}

type ContentCrawler struct {
	client       *http.Client
	domainDelays map[string]time.Time
	visited      map[string]bool
	mutex        sync.RWMutex
}

type ContentType int

const (
	ContentTypeUnknown ContentType = iota
	ContentTypeNews
	ContentTypeBlog
	ContentTypeDocumentation
	ContentTypeProduct
	ContentTypeEducational
	ContentTypeForum
	ContentTypeReference
)

func CreateContentCrawler() *ContentCrawler {
	return &ContentCrawler{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		domainDelays: make(map[string]time.Time),
		visited:      make(map[string]bool),
	}
}

func (c *ContentCrawler) GetQualitySeeds(query string) []SeedURL {
	var seeds []SeedURL

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return seeds
	}

	log.Printf("Getting seeds for query: %s", query)

	intent := c.classifyQueryIntent(normalizedQuery)
	log.Printf("Classified query '%s' as intent: %s", query, intent)

	switch intent {
	case "definition":
		seeds = append(seeds, c.getDefinitionSources(query)...)
	case "factual":
		seeds = append(seeds, c.getFactualSources(query)...)
	case "current_events":
		seeds = append(seeds, c.getCurrentEventsSources(query)...)
	case "how_to":
		seeds = append(seeds, c.getHowToSources(query)...)
	case "academic":
		seeds = append(seeds, c.getAcademicSourcesForQuery(query)...)
	default:
		seeds = append(seeds, c.getGeneralAuthoritativeSources(query)...)
	}

	topicSeeds := c.getTopicSpecificSources(query)
	seeds = append(seeds, topicSeeds...)

	searchEngineSeeds := c.getSearchEngineBasedSeeds(query)
	seeds = append(seeds, searchEngineSeeds...)

	c.sortSeedsByAuthority(seeds)

	if len(seeds) > 15 {
		seeds = seeds[:15]
	}

	log.Printf("Generated %d quality seeds for query: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) classifyQueryIntent(query string) string {
	queryLower := strings.ToLower(query)

	if c.isLiteraryOrCulturalWork(queryLower) {
		return "factual"
	}

	if strings.HasPrefix(queryLower, "what is") || strings.HasPrefix(queryLower, "define") ||
		strings.Contains(queryLower, "meaning") || strings.Contains(queryLower, "definition") {
		return "definition"
	}

	if (strings.HasPrefix(queryLower, "how to") || strings.HasPrefix(queryLower, "how do") ||
		strings.Contains(queryLower, "tutorial") || strings.Contains(queryLower, "guide")) &&
		!c.isLiteraryOrCulturalWork(queryLower) {
		return "how_to"
	}

	if strings.Contains(queryLower, "news") || strings.Contains(queryLower, "latest") ||
		strings.Contains(queryLower, "recent") || strings.Contains(queryLower, "2024") ||
		strings.Contains(queryLower, "2025") || strings.Contains(queryLower, "today") {
		return "current_events"
	}

	if strings.Contains(queryLower, "research") || strings.Contains(queryLower, "study") ||
		strings.Contains(queryLower, "analysis") || strings.Contains(queryLower, "theory") ||
		strings.Contains(queryLower, "paper") {
		return "academic"
	}

	words := strings.Fields(queryLower)
	if len(words) <= 3 {
		return "factual"
	}

	return "general"
}

func (c *ContentCrawler) getDefinitionSources(query string) []SeedURL {
	var seeds []SeedURL

	if c.isSlangOrCulturalQuery(query) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.urbandictionary.com/define.php?term=%s", url.QueryEscape(query)), Priority: 0.95, Topic: "slang", Source: "urbandictionary", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.dictionary.com/browse/%s", url.QueryEscape(query)), Priority: 0.9, Topic: "dictionary", Source: "dictionary", Reliability: 0.95},
			{URL: fmt.Sprintf("https://www.merriam-webster.com/dictionary/%s", url.QueryEscape(query)), Priority: 0.85, Topic: "dictionary", Source: "merriamwebster", Reliability: 0.95},
			{URL: fmt.Sprintf("https://en.wiktionary.org/wiki/%s", url.QueryEscape(query)), Priority: 0.8, Topic: "dictionary", Source: "wiktionary", Reliability: 0.9},
		}...)
	} else {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.merriam-webster.com/dictionary/%s", url.QueryEscape(query)), Priority: 0.95, Topic: "dictionary", Source: "merriamwebster", Reliability: 0.95},
			{URL: fmt.Sprintf("https://www.dictionary.com/browse/%s", url.QueryEscape(query)), Priority: 0.9, Topic: "dictionary", Source: "dictionary", Reliability: 0.95},
			{URL: fmt.Sprintf("https://en.wiktionary.org/wiki/%s", url.QueryEscape(query)), Priority: 0.85, Topic: "dictionary", Source: "wiktionary", Reliability: 0.9},
			{URL: fmt.Sprintf("https://www.oxfordlearnersdictionaries.com/definition/english/%s", url.QueryEscape(query)), Priority: 0.8, Topic: "dictionary", Source: "oxford", Reliability: 0.95},
		}...)
	}

	entity := c.extractEntityFromQuestion(query)
	if entity != "" {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", c.generateWikipediaTitle(entity)), Priority: 0.75, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.95},
		}...)
	}

	return seeds
}

func (c *ContentCrawler) getFactualSources(query string) []SeedURL {
	var seeds []SeedURL

	wikipediaTitle := c.generateWikipediaTitle(query)
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.95, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.95},
		{URL: fmt.Sprintf("https://simple.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.85, Topic: "encyclopedia", Source: "simple_wikipedia", Reliability: 0.9},
	}...)

	if c.isLiteraryOrCulturalWork(query) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.sparknotes.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "literature", Source: "sparknotes", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.cliffsnotes.com/search?query=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "literature", Source: "cliffsnotes", Reliability: 0.8},
			{URL: fmt.Sprintf("https://www.goodreads.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "literature", Source: "goodreads", Reliability: 0.75},
			{URL: fmt.Sprintf("https://www.poetryfoundation.org/search?query=%s", url.QueryEscape(query)), Priority: 0.75, Topic: "literature", Source: "poetry_foundation", Reliability: 0.85},
		}...)
	}

	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "encyclopedia", Source: "britannica", Reliability: 0.95},
		{URL: fmt.Sprintf("https://www.nationalgeographic.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "educational", Source: "natgeo", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.smithsonianmag.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.75, Topic: "educational", Source: "smithsonian", Reliability: 0.9},
	}...)

	return seeds
}

func (c *ContentCrawler) getCurrentEventsSources(query string) []SeedURL {
	return []SeedURL{
		{URL: fmt.Sprintf("https://www.reuters.com/site-search/?query=%s", url.QueryEscape(query)), Priority: 0.95, Topic: "news", Source: "reuters", Reliability: 0.95},
		{URL: fmt.Sprintf("https://www.bbc.com/search?q=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "news", Source: "bbc", Reliability: 0.95},
		{URL: fmt.Sprintf("https://www.npr.org/search?query=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "news", Source: "npr", Reliability: 0.9},
		{URL: fmt.Sprintf("https://apnews.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "news", Source: "ap", Reliability: 0.95},
	}
}

func (c *ContentCrawler) getHowToSources(query string) []SeedURL {
	return []SeedURL{
		{URL: fmt.Sprintf("https://www.wikihow.com/wikiHowTo?search=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "how_to", Source: "wikihow", Reliability: 0.8},
		{URL: fmt.Sprintf("https://www.instructables.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "how_to", Source: "instructables", Reliability: 0.75},
	}
}

func (c *ContentCrawler) getAcademicSourcesForQuery(query string) []SeedURL {
	return []SeedURL{
		{URL: fmt.Sprintf("https://scholar.google.com/scholar?q=%s", url.QueryEscape(query)), Priority: 0.95, Topic: "academic", Source: "google_scholar", Reliability: 0.95},
		{URL: fmt.Sprintf("https://www.jstor.org/action/doBasicSearch?Query=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "academic", Source: "jstor", Reliability: 0.95},
	}
}

func (c *ContentCrawler) getGeneralAuthoritativeSources(query string) []SeedURL {
	var seeds []SeedURL

	wikipediaTitle := c.generateWikipediaTitle(query)
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.9, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.95},
	}...)

	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "encyclopedia", Source: "britannica", Reliability: 0.95},
		{URL: fmt.Sprintf("https://www.theatlantic.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "journalism", Source: "atlantic", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.newyorker.com/search/q/%s", url.QueryEscape(query)), Priority: 0.75, Topic: "journalism", Source: "newyorker", Reliability: 0.9},
	}...)

	return seeds
}

func (c *ContentCrawler) getTopicSpecificSources(query string) []SeedURL {
	var seeds []SeedURL
	queryLower := strings.ToLower(query)

	if c.isTechnicalQuery(queryLower) || strings.Contains(queryLower, "science") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.scientificamerican.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "science", Source: "sciam", Reliability: 0.9},
			{URL: fmt.Sprintf("https://www.nature.com/search?q=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "science", Source: "nature", Reliability: 0.95},
		}...)
	}

	if strings.Contains(queryLower, "health") || strings.Contains(queryLower, "medical") ||
		strings.Contains(queryLower, "disease") || strings.Contains(queryLower, "symptoms") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.mayoclinic.org/search/search-results?q=%s", url.QueryEscape(query)), Priority: 0.95, Topic: "health", Source: "mayoclinic", Reliability: 0.95},
			{URL: fmt.Sprintf("https://www.webmd.com/search/search_results/default.aspx?query=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "health", Source: "webmd", Reliability: 0.8},
		}...)
	}

	if strings.Contains(queryLower, "history") || strings.Contains(queryLower, "historical") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.history.com/search?q=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "history", Source: "history", Reliability: 0.85},
		}...)
	}

	return seeds
}

func (c *ContentCrawler) getSearchEngineBasedSeeds(query string) []SeedURL {
	return []SeedURL{
		{URL: fmt.Sprintf("https://duckduckgo.com/?q=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "search", Source: "duckduckgo", Reliability: 0.7},
	}
}

func (c *ContentCrawler) sortSeedsByAuthority(seeds []SeedURL) {
	sort.Slice(seeds, func(i, j int) bool {
		if seeds[i].Priority != seeds[j].Priority {
			return seeds[i].Priority > seeds[j].Priority
		}
		return seeds[i].Reliability > seeds[j].Reliability
	})
}

func (c *ContentCrawler) isTechnicalQuery(query string) bool {
	technicalKeywords := []string{
		"programming", "code", "software", "development", "api", "framework",
		"library", "documentation", "tutorial", "how to code", "algorithm",
		"database", "javascript", "python", "java", "html", "css", "react",
		"node", "git", "github", "docker", "kubernetes", "aws", "cloud",
		"machine learning", "ai", "data science", "web development",
	}

	for _, keyword := range technicalKeywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}

func (c *ContentCrawler) CrawlContent(seeds []SeedURL) []CrawledDocument {
	var documents []CrawledDocument
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 3)

	for _, seed := range seeds {
		wg.Add(1)
		go func(s SeedURL) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			doc := c.crawlSinglePage(s)
			if doc != nil && c.isQualityContent(doc) {
				mu.Lock()
				documents = append(documents, *doc)
				mu.Unlock()
				log.Printf("Successfully crawled quality content from: %s (quality: %.2f)", s.URL, doc.Quality)
			}
		}(seed)
	}

	wg.Wait()
	close(semaphore)

	return documents
}

func (c *ContentCrawler) crawlSinglePage(seed SeedURL) *CrawledDocument {
	c.respectDomainDelay(seed.URL)

	c.mutex.Lock()
	if c.visited[seed.URL] {
		c.mutex.Unlock()
		return nil
	}
	c.visited[seed.URL] = true
	c.mutex.Unlock()

	log.Printf("Crawling: %s", seed.URL)

	resp, err := c.client.Get(seed.URL)
	if err != nil {
		log.Printf("Error fetching %s: %v", seed.URL, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Non-200 status for %s: %d", seed.URL, resp.StatusCode)
		return nil
	}

	if !c.isValidContentType(resp.Header.Get("Content-Type")) {
		log.Printf("Invalid content type for %s: %s", seed.URL, resp.Header.Get("Content-Type"))
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading body for %s: %v", seed.URL, err)
		return nil
	}

	html := string(body)
	log.Printf("Received HTML content length: %d bytes for %s", len(html), seed.URL)

	if strings.Contains(seed.URL, "wikipedia.org") && strings.Contains(seed.URL, "Special:Search") {
		articleURL := c.extractWikipediaArticleFromSearch(html, seed.Topic)
		if articleURL != "" {
			log.Printf("Found actual Wikipedia article: %s", articleURL)
			newSeed := SeedURL{
				URL:         articleURL,
				Priority:    seed.Priority,
				Topic:       seed.Topic,
				Source:      seed.Source,
				Reliability: seed.Reliability,
			}
			return c.crawlSinglePage(newSeed)
		}
	}

	content := c.extractMainContent(html)
	log.Printf("Main content extraction returned %d characters", len(content))
	if content == "" {
		content = c.extractCleanContent(html)
		log.Printf("Clean content extraction returned %d characters", len(content))
	}

	if len(content) < 100 {
		content = c.extractParagraphContent(html)
		log.Printf("Paragraph content extraction returned %d characters", len(content))
	}

	if len(content) < 100 {
		content = c.extractSimpleTextContent(html)
		log.Printf("Simple text extraction returned %d characters", len(content))
	}

	if len(content) < 100 {
		log.Printf("Content too short for %s: %d characters", seed.URL, len(content))
		return nil
	}

	parsedURL, _ := url.Parse(seed.URL)
	domain := ""
	if parsedURL != nil {
		domain = parsedURL.Host
	}

	doc := &CrawledDocument{
		URL:         seed.URL,
		Title:       c.extractTitle(html),
		Content:     content,
		Description: c.extractDescription(html),
		Keywords:    c.extractKeywords(html),
		Links:       c.extractLinks(html, seed.URL),
		Language:    c.detectLanguage(content),
		Quality:     seed.Reliability,
		CrawledAt:   time.Now(),
		Size:        len(content),
		Domain:      domain,
	}

	log.Printf("Created document for %s: title='%s', content_length=%d, word_count=%d",
		seed.URL, doc.Title, len(doc.Content), len(strings.Fields(doc.Content)))

	return doc
}

func (c *ContentCrawler) extractCleanContent(html string) string {
	html = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<nav[^>]*>.*?</nav>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<header[^>]*>.*?</header>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<footer[^>]*>.*?</footer>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<aside[^>]*>.*?</aside>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<!--.*?-->`).ReplaceAllString(html, "")

	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`&[a-zA-Z0-9#]+;`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")

	content := strings.TrimSpace(html)
	content = c.removeNavigationText(content)

	return content
}

func (c *ContentCrawler) extractMainContent(html string) string {
	patterns := []string{
		`(?is)<div[^>]*class="[^"]*mw-parser-output[^"]*"[^>]*>(.*?)</div>`,
		`(?is)<div[^>]*class="[^"]*mw-content-text[^"]*"[^>]*>(.*?)</div>`,
		`(?is)<main[^>]*>(.*?)</main>`,
		`(?is)<article[^>]*>(.*?)</article>`,
		`(?is)<div[^>]*class="[^"]*content[^"]*"[^>]*>(.*?)</div>`,
		`(?is)<div[^>]*id="[^"]*content[^"]*"[^>]*>(.*?)</div>`,
		`(?is)<div[^>]*class="[^"]*main[^"]*"[^>]*>(.*?)</div>`,
		`(?is)<div[^>]*id="[^"]*main[^"]*"[^>]*>(.*?)</div>`,
		`(?is)<section[^>]*class="[^"]*content[^"]*"[^>]*>(.*?)</section>`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			content := c.extractCleanContent(matches[1])
			if len(content) > 200 {
				return content
			}
		}
	}

	return ""
}

func (c *ContentCrawler) extractParagraphContent(html string) string {
	html = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<nav[^>]*>.*?</nav>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<header[^>]*>.*?</header>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)<footer[^>]*>.*?</footer>`).ReplaceAllString(html, "")

	paragraphRegex := regexp.MustCompile(`<p[^>]*>(.*?)</p>`)
	matches := paragraphRegex.FindAllStringSubmatch(html, -1)

	log.Printf("Found %d paragraph matches", len(matches))

	var paragraphs []string
	for i, match := range matches {
		if len(match) > 1 {
			p := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(match[1], " ")
			p = regexp.MustCompile(`&[a-zA-Z0-9#]+;`).ReplaceAllString(p, " ")
			p = regexp.MustCompile(`\s+`).ReplaceAllString(p, " ")
			p = strings.TrimSpace(p)
			if len(p) > 50 {
				paragraphs = append(paragraphs, p)
				if i < 3 {
					log.Printf("Sample paragraph %d: %.100s...", i+1, p)
				}
			}
		}
	}

	content := strings.Join(paragraphs, "\n\n")
	log.Printf("Extracted %d paragraphs, total length: %d", len(paragraphs), len(content))
	return c.removeNavigationText(content)
}

func (c *ContentCrawler) extractTitle(html string) string {
	re := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		title = regexp.MustCompile(`&[a-zA-Z0-9#]+;`).ReplaceAllString(title, " ")
		title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
		return title
	}
	return "Untitled"
}

func (c *ContentCrawler) extractDescription(html string) string {
	patterns := []string{
		`(?i)<meta[^>]*name="description"[^>]*content="([^"]*)"`,
		`(?i)<meta[^>]*content="([^"]*)"[^>]*name="description"`,
		`(?i)<meta[^>]*property="og:description"[^>]*content="([^"]*)"`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 && len(strings.TrimSpace(matches[1])) > 0 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func (c *ContentCrawler) extractKeywords(html string) []string {
	patterns := []string{
		`(?i)<meta[^>]*name="keywords"[^>]*content="([^"]*)"`,
		`(?i)<meta[^>]*content="([^"]*)"[^>]*name="keywords"`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			keywords := strings.Split(matches[1], ",")
			var cleanKeywords []string
			for _, kw := range keywords {
				kw = strings.TrimSpace(kw)
				if len(kw) > 0 {
					cleanKeywords = append(cleanKeywords, kw)
				}
			}
			return cleanKeywords
		}
	}
	return []string{}
}

func (c *ContentCrawler) extractLinks(html, baseURL string) []string {
	var links []string
	base, err := url.Parse(baseURL)
	if err != nil {
		return links
	}

	re := regexp.MustCompile(`(?i)<a[^>]*href="([^"]*)"`)
	matches := re.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(match) > 1 {
			href := c.resolveURL(match[1], base)
			if href != "" && !strings.Contains(href, "#") {
				links = append(links, href)
			}
		}
	}

	return links
}

func (c *ContentCrawler) resolveURL(href string, base *url.URL) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(u)
	if resolved.Scheme == "http" || resolved.Scheme == "https" {
		return resolved.String()
	}
	return ""
}

func (c *ContentCrawler) detectLanguage(content string) string {
	if len(content) < 50 {
		return "unknown"
	}

	sampleSize := 1000
	if len(content) < sampleSize {
		sampleSize = len(content)
	}
	sample := strings.ToLower(content[:sampleSize])

	englishWords := []string{"the", "and", "for", "are", "but", "not", "you", "all", "can", "had", "her", "was", "one", "our", "out", "day", "get", "has", "him", "his", "how", "its", "new", "now", "old", "see", "two", "way", "who", "boy", "did", "man", "men", "put", "say", "she", "too", "use"}

	englishCount := 0
	totalWords := len(strings.Fields(sample))

	if totalWords == 0 {
		return "unknown"
	}

	for _, word := range englishWords {
		if strings.Contains(sample, " "+word+" ") || strings.HasPrefix(sample, word+" ") || strings.HasSuffix(sample, " "+word) {
			englishCount++
		}
	}

	if float64(englishCount)/float64(len(englishWords)) > 0.3 {
		return "en"
	}

	return "unknown"
}

func (c *ContentCrawler) removeNavigationText(content string) string {
	navPatterns := []string{
		`(?i)\b(home|about|contact|search|login|register|sign in|sign up|menu|navigation|nav|breadcrumb)\b`,
		`(?i)\b(next|previous|prev|back|forward|more|less|expand|collapse)\b`,
		`(?i)\b(privacy policy|terms of service|cookie policy|disclaimer)\b`,
		`(?i)\b(follow us|social media|share|like|tweet|facebook|twitter|instagram)\b`,
		`(?i)\b(advertisement|ads|sponsored|related articles|you might also like)\b`,
	}

	lines := strings.Split(content, "\n")
	var cleanLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 10 {
			continue
		}

		isNav := false
		for _, pattern := range navPatterns {
			re := regexp.MustCompile(pattern)
			if re.MatchString(line) {
				isNav = true
				break
			}
		}

		if !isNav {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

func (c *ContentCrawler) isQualityContent(doc *CrawledDocument) bool {
	if doc == nil {
		log.Printf("Quality check failed: document is nil")
		return false
	}

	if len(doc.Content) < 200 {
		log.Printf("Quality check failed for %s: content too short (%d chars)", doc.URL, len(doc.Content))
		return false
	}

	if len(doc.Title) < 5 {
		log.Printf("Quality check failed for %s: title too short ('%s')", doc.URL, doc.Title)
		return false
	}

	wordCount := len(strings.Fields(doc.Content))
	if wordCount < 50 {
		log.Printf("Quality check failed for %s: too few words (%d)", doc.URL, wordCount)
		return false
	}

	avgWordLength := float64(len(doc.Content)) / float64(wordCount)
	maxAvgWordLength := 20.0
	if strings.Contains(doc.URL, "wikihow.com") || strings.Contains(doc.URL, "instructables.com") ||
		strings.Contains(doc.URL, "wikipedia.org") || strings.Contains(doc.URL, "educational") {
		maxAvgWordLength = 30.0
	}

	if avgWordLength < 3.0 || avgWordLength > maxAvgWordLength {
		log.Printf("Quality check failed for %s: suspicious average word length (%.2f, max: %.2f)", doc.URL, avgWordLength, maxAvgWordLength)
		return false
	}

	if strings.Contains(strings.ToLower(doc.Content), "javascript") && strings.Contains(strings.ToLower(doc.Content), "enable") {
		return false
	}

	if strings.Contains(doc.URL, "github.com") {
		titleLower := strings.ToLower(doc.Title)
		contentLower := strings.ToLower(doc.Content)

		if strings.Contains(titleLower, "github") &&
			(strings.Contains(contentLower, "repositories 0") ||
				strings.Contains(contentLower, "dismiss alert") ||
				strings.Contains(contentLower, "i love")) {
			return false
		}
	}

	lowQualityIndicators := []string{
		"dismiss alert", "repositories 0", "catboy", "instagram",
		"tiktok", "@ooo", "coming soon", "under construction",
		"lorem ipsum", "test page", "placeholder", "example",
	}
	contentLower := strings.ToLower(doc.Content)
	titleLower := strings.ToLower(doc.Title)

	emptySearchIndicators := []string{
		"we couldn't find a match",
		"no results found",
		"no matches found",
		"couldn't find a match for",
		"no results for",
		"sorry, there are no results",
		"your search did not match",
		"did you mean:",
		"showing results for",
		"try different keywords",
		"no content available",
		"search returned no results",
		"no matches were found",
		"no entries found",
		"word not found",
		"definition not found",
		"no definition available",
		"search suggestions",
		"related searches",
		"alternate searches",
		"similar searches",
		"suggested searches",
		"spell check",
		"correct spelling",
		"nothing here yet",
		"page not found",
		"could not be found",
		"not available",
		"coming soon",
		"under construction",
		"temporarily unavailable",
	}

	for _, indicator := range emptySearchIndicators {
		if strings.Contains(contentLower, indicator) {
			log.Printf("Filtering out empty search page: %s (contains: %s)", doc.URL, indicator)
			return false
		}
	}

	if strings.Contains(doc.URL, "nationalgeographic.com") || strings.Contains(doc.URL, "britannica.com") {
		if strings.Contains(contentLower, "search results") && wordCount < 150 {
			log.Printf("Filtering out thin search results page: %s", doc.URL)
			return false
		}
		if strings.Contains(titleLower, "search") && !strings.Contains(contentLower, "encyclopedia") {
			log.Printf("Filtering out search page without content: %s", doc.URL)
			return false
		}
	}

	if strings.Contains(doc.URL, "search") || strings.Contains(titleLower, "search") {
		if wordCount < 100 || len(doc.Content) < 500 {
			log.Printf("Filtering out thin search page: %s (words: %d, chars: %d)", doc.URL, wordCount, len(doc.Content))
			return false
		}
	}

	for _, indicator := range lowQualityIndicators {
		if strings.Contains(contentLower, indicator) || strings.Contains(titleLower, indicator) {
			return false
		}
	}

	spamIndicators := []string{"click here", "buy now", "limited time", "act fast", "100% free", "guarantee"}
	spamCount := 0
	for _, indicator := range spamIndicators {
		if strings.Contains(contentLower, indicator) {
			spamCount++
		}
	}
	return spamCount < 3
}

func (c *ContentCrawler) isValidContentType(contentType string) bool {
	validTypes := []string{
		"text/html",
		"application/xhtml+xml",
		"text/plain",
	}

	for _, validType := range validTypes {
		if strings.Contains(contentType, validType) {
			return true
		}
	}

	return false
}

func (c *ContentCrawler) respectDomainDelay(urlStr string) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return
	}

	domain := parsedURL.Host

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if lastAccess, exists := c.domainDelays[domain]; exists {
		elapsed := time.Since(lastAccess)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}

	c.domainDelays[domain] = time.Now()
}

func (c *ContentCrawler) generateWikipediaTitle(query string) string {
	title := strings.TrimSpace(query)

	words := strings.Fields(title)
	for i, word := range words {
		if len(word) > 0 {
			if i == 0 || !isSmallWord(word) {
				words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
			} else {
				words[i] = strings.ToLower(word)
			}
		}
	}

	result := strings.Join(words, "_")

	result = strings.ReplaceAll(result, " ", "_")

	return result
}

func isSmallWord(word string) bool {
	smallWords := map[string]bool{
		"a": true, "an": true, "and": true, "as": true, "at": true, "but": true,
		"by": true, "for": true, "if": true, "in": true, "of": true, "on": true,
		"or": true, "the": true, "to": true, "up": true, "via": true, "is": true,
	}
	return smallWords[strings.ToLower(word)]
}

func (c *ContentCrawler) extractWikipediaArticleFromSearch(html, originalQuery string) string {
	re := regexp.MustCompile(`<a[^>]*href="(/wiki/[^"#]*)"[^>]*title="([^"]*)"`)
	matches := re.FindAllStringSubmatch(html, -1)

	queryLower := strings.ToLower(originalQuery)

	for _, match := range matches {
		if len(match) >= 3 {
			href := match[1]
			title := strings.ToLower(match[2])

			if strings.Contains(href, ":") || strings.Contains(title, "disambiguation") {
				continue
			}

			if c.isRelevantWikipediaResult(title, queryLower) {
				return "https://en.wikipedia.org" + href
			}
		}
	}

	return ""
}

func (c *ContentCrawler) isRelevantWikipediaResult(title, query string) bool {
	queryWords := strings.Fields(query)
	titleWords := strings.Fields(title)

	matches := 0
	for _, qWord := range queryWords {
		if len(qWord) < 3 {
			continue
		}
		for _, tWord := range titleWords {
			if strings.Contains(tWord, qWord) || strings.Contains(qWord, tWord) {
				matches++
				break
			}
		}
	}

	return matches >= len(queryWords)/2 && matches > 0
}

func (c *ContentCrawler) extractSimpleTextContent(html string) string {
	content := html
	content = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<nav[^>]*>.*?</nav>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<header[^>]*>.*?</header>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<footer[^>]*>.*?</footer>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<!--.*?-->`).ReplaceAllString(content, "")

	content = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(content, " ")
	content = regexp.MustCompile(`https?://[^\s]+`).ReplaceAllString(content, " ")
	content = regexp.MustCompile(`www\.[^\s]+`).ReplaceAllString(content, " ")
	content = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`).ReplaceAllString(content, " ")
	content = regexp.MustCompile(`[a-zA-Z0-9]{20,}`).ReplaceAllString(content, " ")
	content = regexp.MustCompile(`\b[A-Z0-9]{10,}\b`).ReplaceAllString(content, " ")

	content = regexp.MustCompile(`&amp;`).ReplaceAllString(content, "&")
	content = regexp.MustCompile(`&lt;`).ReplaceAllString(content, "<")
	content = regexp.MustCompile(`&gt;`).ReplaceAllString(content, ">")
	content = regexp.MustCompile(`&quot;`).ReplaceAllString(content, "\"")
	content = regexp.MustCompile(`&#39;`).ReplaceAllString(content, "'")
	content = regexp.MustCompile(`&nbsp;`).ReplaceAllString(content, " ")

	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
	content = strings.TrimSpace(content)

	sentences := regexp.MustCompile(`[.!?]+`).Split(content, -1)
	var validSentences []string

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) > 20 && len(sentence) < 500 {
			lower := strings.ToLower(sentence)
			if !strings.Contains(lower, "cookie") &&
				!strings.Contains(lower, "privacy policy") &&
				!strings.Contains(lower, "sign in") &&
				!strings.Contains(lower, "subscribe") &&
				!strings.Contains(lower, "advertisement") {
				validSentences = append(validSentences, sentence)
			}
		}
	}

	result := strings.Join(validSentences, ". ")
	if len(result) > 10000 {
		result = result[:10000] + "..."
	}

	return result
}

func (c *ContentCrawler) extractEntityFromQuestion(query string) string {
	patterns := []struct {
		prefix   string
		suffixes []string
	}{
		{"what is ", []string{"", " about", " like", " exactly"}},
		{"who is ", []string{"", " exactly", " really"}},
		{"what are ", []string{"", " about", " like"}},
		{"how to ", []string{"", " effectively", " properly", " safely"}},
		{"how do i ", []string{"", " effectively", " properly", " safely"}},
		{"how can i ", []string{"", " effectively", " properly", " safely"}},
		{"tell me about ", []string{""}},
		{"information about ", []string{""}},
		{"about ", []string{""}},
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))

	for _, pattern := range patterns {
		if strings.HasPrefix(queryLower, pattern.prefix) {
			entity := strings.TrimPrefix(queryLower, pattern.prefix)

			for _, suffix := range pattern.suffixes {
				if suffix != "" && strings.HasSuffix(entity, suffix) {
					entity = strings.TrimSuffix(entity, suffix)
				}
			}

			entity = strings.TrimSpace(entity)

			articles := []string{"a ", "an ", "the "}
			for _, article := range articles {
				if strings.HasPrefix(entity, article) {
					entity = strings.TrimPrefix(entity, article)
					entity = strings.TrimSpace(entity)
					break
				}
			}

			if c.isValidEntity(entity) {
				log.Printf("Extracted entity '%s' from question: %s", entity, query)
				return entity
			}
		}
	}

	return ""
}

func (c *ContentCrawler) isValidEntity(entity string) bool {
	words := strings.Fields(entity)

	if len(words) == 0 || len(words) > 4 {
		return false
	}

	remainingQuestionWords := []string{"how", "why", "when", "where", "which", "can", "do", "does", "did"}
	for _, word := range words {
		for _, qw := range remainingQuestionWords {
			if strings.Contains(word, qw) {
				return false
			}
		}
	}

	if len(entity) < 2 || len(entity) > 50 {
		return false
	}

	return true
}

func (c *ContentCrawler) isSlangOrCulturalQuery(query string) bool {
	slangTerms := []string{
		"rizz", "simp", "salty", "cap", "no cap", "bet", "slaps", "bussin", "fire",
		"mid", "periodt", "stan", "ship", "sus", "vibe", "mood", "flex", "slay",
		"tea", "spill the tea", "lowkey", "highkey", "basic", "karen", "boomer",
		"millennial", "gen z", "cringe", "based", "woke", "cancelled", "ghosted",
		"slide into dms", "thirsty", "extra", "savage", "lit", "fam", "bae",
		"thicc", "yeet", "oof", "bruh", "periodt", "periodt", "facts", "deadass",
		"fr", "frfr", "ong", "ngl", "tbh", "smh", "iykyk", "main character",
		"red flag", "green flag", "gaslighting", "toxic", "manifesting",
		"cheugy", "sheesh", "sending me", "not me", "i'm deceased", "say less",
	}

	culturalTerms := []string{
		"meme", "viral", "trending", "hashtag", "influencer", "tiktoker",
		"youtuber", "streamer", "content creator", "social media", "going viral",
		"internet culture", "online slang", "digital native", "screen time",
		"doomscrolling", "parasocial", "finsta", "spam account", "story time",
		"get ready with me", "outfit of the day", "daily vlog", "unboxing",
	}

	for _, term := range slangTerms {
		if strings.Contains(query, term) {
			return true
		}
	}

	for _, term := range culturalTerms {
		if strings.Contains(query, term) {
			return true
		}
	}

	if strings.HasPrefix(query, "what is ") {
		words := strings.Fields(query)
		if len(words) == 3 && len(words[2]) <= 8 {
			return true
		}
	}

	return false
}

func (c *ContentCrawler) isLiteraryOrCulturalWork(query string) bool {
	literaryWorks := []string{
		"to kill a mockingbird", "how to kill a mockingbird",
		"pride and prejudice", "1984", "animal farm", "brave new world",
		"the great gatsby", "lord of the flies", "of mice and men",
		"the catcher in the rye", "jane eyre", "wuthering heights",
		"frankenstein", "dracula", "the hobbit", "lord of the rings",
		"harry potter", "game of thrones", "the hunger games",
		"fifty shades", "twilight", "the da vinci code",
		"gone girl", "the girl with the dragon tattoo",
		"how to win friends and influence people",
		"how to be a woman", "how to train your dragon",
		"the art of war", "the prince", "the republic",
		"a brief history of time", "sapiens", "homo deus",
	}

	queryLower := strings.ToLower(query)

	for _, work := range literaryWorks {
		if strings.Contains(queryLower, work) {
			return true
		}
	}

	if strings.Contains(queryLower, "book") || strings.Contains(queryLower, "novel") ||
		strings.Contains(queryLower, "author") || strings.Contains(queryLower, "chapter") {
		return true
	}

	return false
}
