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

func NewContentCrawler() *ContentCrawler {
	return &ContentCrawler{
		client: &http.Client{
			Timeout: 15 * time.Second,
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

	// Generate actual Wikipedia article URLs instead of search URLs
	wikipediaTitle := c.generateWikipediaTitle(query)

	if c.isScientificTopic(query) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.9, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "encyclopedia", Source: "britannica", Reliability: 0.9},
			{URL: fmt.Sprintf("https://www.nationalgeographic.com/search?q=%s", url.QueryEscape(query)), Priority: 0.75, Topic: "science", Source: "natgeo", Reliability: 0.8},
		}...)
	}

	if c.isTechnologyTopic(query) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://developer.mozilla.org/en-US/search?q=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "documentation", Source: "mdn", Reliability: 0.95},
			{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.8, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.85},
			{URL: fmt.Sprintf("https://stackoverflow.com/search?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "qa", Source: "stackoverflow", Reliability: 0.75},
		}...)
	}

	if c.isHistoricalTopic(query) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.9, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "encyclopedia", Source: "britannica", Reliability: 0.9},
			{URL: fmt.Sprintf("https://www.history.com/search?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "history", Source: "history", Reliability: 0.75},
		}...)
	}

	if c.isPoliticalOrLegalTopic(query) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.9, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.85, Topic: "encyclopedia", Source: "britannica", Reliability: 0.9},
		}...)
	}

	// Always add general Wikipedia and other sources
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.8, Topic: "general", Source: "wikipedia", Reliability: 0.85},
		{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.75, Topic: "general", Source: "britannica", Reliability: 0.9},
	}...)

	// Add fallback search URLs in case direct article URLs fail
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/Special:Search?search=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "search", Source: "wikipedia_search", Reliability: 0.7},
	}...)

	c.sortSeedsByPriority(seeds)

	if len(seeds) > 10 {
		seeds = seeds[:10]
	}

	return seeds
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

	// Check if this is a Wikipedia search page and try to extract actual article
	if strings.Contains(seed.URL, "wikipedia.org") && strings.Contains(seed.URL, "Special:Search") {
		articleURL := c.extractWikipediaArticleFromSearch(html, seed.Topic)
		if articleURL != "" {
			log.Printf("Found actual Wikipedia article: %s", articleURL)
			// Create a new seed for the actual article and crawl it
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
	if content == "" {
		content = c.extractCleanContent(html)
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
		`(?i)<main[^>]*>(.*?)</main>`,
		`(?i)<article[^>]*>(.*?)</article>`,
		`(?i)<div[^>]*class="[^"]*content[^"]*"[^>]*>(.*?)</div>`,
		`(?i)<div[^>]*id="[^"]*content[^"]*"[^>]*>(.*?)</div>`,
		`(?i)<div[^>]*class="[^"]*main[^"]*"[^>]*>(.*?)</div>`,
		`(?i)<div[^>]*id="[^"]*main[^"]*"[^>]*>(.*?)</div>`,
		`(?i)<section[^>]*class="[^"]*content[^"]*"[^>]*>(.*?)</section>`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 && len(strings.TrimSpace(matches[1])) > 200 {
			content := c.extractCleanContent(matches[1])
			if len(content) > 200 {
				return content
			}
		}
	}

	return ""
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
		return false
	}

	if len(doc.Content) < 200 {
		return false
	}

	if len(doc.Title) < 5 {
		return false
	}

	wordCount := len(strings.Fields(doc.Content))
	if wordCount < 50 {
		return false
	}

	avgWordLength := float64(len(doc.Content)) / float64(wordCount)
	if avgWordLength < 3.0 || avgWordLength > 20.0 {
		return false
	}

	if strings.Contains(strings.ToLower(doc.Content), "javascript") && strings.Contains(strings.ToLower(doc.Content), "enable") {
		return false
	}

	spamIndicators := []string{"click here", "buy now", "limited time", "act fast", "100% free", "guarantee"}
	contentLower := strings.ToLower(doc.Content)
	spamCount := 0
	for _, indicator := range spamIndicators {
		if strings.Contains(contentLower, indicator) {
			spamCount++
		}
	}
	if spamCount >= 3 {
		return false
	}

	return true
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

func (c *ContentCrawler) sortSeedsByPriority(seeds []SeedURL) {
	sort.Slice(seeds, func(i, j int) bool {
		return seeds[i].Priority > seeds[j].Priority
	})
}

func (c *ContentCrawler) isScientificTopic(query string) bool {
	scientificTerms := []string{"biology", "chemistry", "physics", "mathematics", "astronomy", "geology", "ecology", "genetics", "evolution", "molecule", "atom", "quantum", "theory", "hypothesis", "experiment", "research", "study", "analysis", "data", "scientific"}
	queryLower := strings.ToLower(query)
	for _, term := range scientificTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}
	return false
}

func (c *ContentCrawler) isTechnologyTopic(query string) bool {
	techTerms := []string{"programming", "software", "computer", "algorithm", "code", "development", "web", "app", "database", "network", "security", "javascript", "python", "java", "html", "css", "framework", "library", "api", "technology", "digital", "internet"}
	queryLower := strings.ToLower(query)
	for _, term := range techTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}
	return false
}

func (c *ContentCrawler) isHistoricalTopic(query string) bool {
	historicalTerms := []string{"history", "historical", "ancient", "medieval", "renaissance", "war", "empire", "civilization", "dynasty", "revolution", "century", "period", "era", "timeline", "archaeological", "artifact", "culture", "tradition", "heritage"}
	queryLower := strings.ToLower(query)
	for _, term := range historicalTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}
	return false
}

func (c *ContentCrawler) isPoliticalOrLegalTopic(query string) bool {
	politicalTerms := []string{"politics", "government", "law", "legal", "constitution", "democracy", "election", "vote", "policy", "legislation", "parliament", "congress", "court", "justice", "rights", "freedom", "liberty", "citizenship", "sovereignty"}
	queryLower := strings.ToLower(query)
	for _, term := range politicalTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}
	return false
}

func (c *ContentCrawler) generateWikipediaTitle(query string) string {
	// Clean and format the query for Wikipedia article URLs
	title := strings.TrimSpace(query)

	// Convert to title case for better Wikipedia matching
	words := strings.Fields(title)
	for i, word := range words {
		if len(word) > 0 {
			// Don't capitalize small words unless they're the first word
			if i == 0 || !isSmallWord(word) {
				words[i] = strings.Title(strings.ToLower(word))
			} else {
				words[i] = strings.ToLower(word)
			}
		}
	}

	// Join with underscores (Wikipedia format)
	result := strings.Join(words, "_")

	// URL encode only special characters, but keep underscores
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
	// Extract actual Wikipedia article URL from search results
	re := regexp.MustCompile(`<a[^>]*href="(/wiki/[^"#]*)"[^>]*title="([^"]*)"`)
	matches := re.FindAllStringSubmatch(html, -1)

	queryLower := strings.ToLower(originalQuery)

	for _, match := range matches {
		if len(match) >= 3 {
			href := match[1]
			title := strings.ToLower(match[2])

			// Skip meta pages and disambiguation
			if strings.Contains(href, ":") || strings.Contains(title, "disambiguation") {
				continue
			}

			// Check if this article title relates to our query
			if c.isRelevantWikipediaResult(title, queryLower) {
				return "https://en.wikipedia.org" + href
			}
		}
	}

	return ""
}

func (c *ContentCrawler) isRelevantWikipediaResult(title, query string) bool {
	// Simple relevance check
	queryWords := strings.Fields(query)
	titleWords := strings.Fields(title)

	matches := 0
	for _, qWord := range queryWords {
		if len(qWord) < 3 { // Skip short words
			continue
		}
		for _, tWord := range titleWords {
			if strings.Contains(tWord, qWord) || strings.Contains(qWord, tWord) {
				matches++
				break
			}
		}
	}

	// Consider relevant if at least half the meaningful words match
	return matches >= len(queryWords)/2 && matches > 0
}
