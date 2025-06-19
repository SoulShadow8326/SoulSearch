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

	// Check if this is a question about a general topic
	extractedEntity := c.extractEntityFromQuestion(query)
	if extractedEntity != "" {
		log.Printf("Extracted entity '%s' from question query", extractedEntity)

		// For questions like "what is a duck", prioritize educational sources
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", c.generateWikipediaTitle(extractedEntity)), Priority: 0.9, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.95},
			{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(extractedEntity)), Priority: 0.8, Topic: "encyclopedia", Source: "britannica", Reliability: 0.9},
			{URL: fmt.Sprintf("https://simple.wikipedia.org/wiki/%s", c.generateWikipediaTitle(extractedEntity)), Priority: 0.7, Topic: "encyclopedia", Source: "simple_wikipedia", Reliability: 0.85},
		}...)

		// Add some general knowledge sources
		seeds = append(seeds, c.getGeneralKnowledgeSources(extractedEntity)...)
	} else {
		// Check if this is a simple general knowledge query (single word or simple phrase)
		if c.isGeneralKnowledgeQuery(query) {
			log.Printf("Treating '%s' as general knowledge query", query)
			// Prioritize Wikipedia and educational sources for general topics
			wikipediaTitle := c.generateWikipediaTitle(query)
			seeds = append(seeds, []SeedURL{
				{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.9, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.95},
				{URL: fmt.Sprintf("https://simple.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.8, Topic: "encyclopedia", Source: "simple_wikipedia", Reliability: 0.9},
				{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "encyclopedia", Source: "britannica", Reliability: 0.9},
			}...)

			// Add some general knowledge sources
			seeds = append(seeds, c.getGeneralKnowledgeSources(query)...)
		} else {
			// Check if this is an entity-specific query
			entitySeeds := c.getEntitySpecificSources(query)
			if len(entitySeeds) > 0 {
				log.Printf("Using entity-specific sources for: %s", query)
				seeds = append(seeds, entitySeeds...)
			}

			// Add Wikipedia and other reliable sources as backup
			wikipediaTitle := c.generateWikipediaTitle(query)
			seeds = append(seeds, []SeedURL{
				{URL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", wikipediaTitle), Priority: 0.6, Topic: "encyclopedia", Source: "wikipedia", Reliability: 0.85},
				{URL: fmt.Sprintf("https://www.britannica.com/search?query=%s", url.QueryEscape(query)), Priority: 0.5, Topic: "encyclopedia", Source: "britannica", Reliability: 0.9},
			}...)
		}
	}

	// For now, use only direct sources to avoid external API issues
	directSeeds := c.getDirectSources(query)
	seeds = append(seeds, directSeeds...)

	c.sortSeedsByPriority(seeds)

	// Limit to avoid overwhelming the system
	if len(seeds) > 12 {
		seeds = seeds[:12]
	}

	log.Printf("Final seed count: %d for query: %s", len(seeds), query)
	for i, seed := range seeds {
		log.Printf("Seed %d: %s (priority: %.2f, source: %s)", i+1, seed.URL, seed.Priority, seed.Source)
	}
	return seeds
}

func (c *ContentCrawler) getSearchEngineResults(query string) []SeedURL {
	var seeds []SeedURL

	// Use only the most reliable search engines to avoid timeouts
	log.Printf("Searching for diverse web content for query: %s", query)

	// Try DuckDuckGo first (most reliable)
	duckDuckGoSeeds := c.extractRealURLsFromDuckDuckGo(query)
	seeds = append(seeds, duckDuckGoSeeds...)

	// If we don't have enough results, try one more search engine
	if len(seeds) < 8 {
		// Try Bing for different perspectives
		bingSeeds := c.extractURLsFromBing(query)
		seeds = append(seeds, bingSeeds...)
	}

	// Add some high-quality direct sources if still low
	if len(seeds) < 5 {
		aggregatorSeeds := c.getContentAggregatorSeeds(query)
		seeds = append(seeds, aggregatorSeeds...)
	}

	log.Printf("Total search engine seeds collected: %d", len(seeds))
	return seeds
}

func (c *ContentCrawler) extractRealURLsFromDuckDuckGo(query string) []SeedURL {
	var seeds []SeedURL

	// Use only the most reliable DuckDuckGo interface to avoid timeouts
	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(query))
	log.Printf("Trying DuckDuckGo: %s", searchURL)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		log.Printf("Error creating DuckDuckGo request: %v", err)
		return seeds
	}

	// Add headers to appear more like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Error fetching DuckDuckGo results: %v", err)
		return seeds
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Non-200 status from DDG: %d", resp.StatusCode)
		return seeds
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading DuckDuckGo response: %v", err)
		return seeds
	}

	html := string(body)
	log.Printf("DDG response length: %d chars", len(html))

	// Multiple patterns to extract URLs from DuckDuckGo results
	patterns := []string{
		// Standard result links
		`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>([^<]*)</a>`,
		// Alternative result patterns
		`<a[^>]*href="https://duckduckgo\.com/l/\?uddg=([^"&]+)[^"]*"[^>]*>([^<]*)</a>`,
		// Direct links
		`<a[^>]*href="(https?://[^"]+)"[^>]*class="[^"]*result[^"]*"[^>]*>([^<]*)</a>`,
		// Backup pattern for any external links
		`<a[^>]*href="(https?://[^"]+)"[^>]*>([^<]*)</a>`,
	}

	foundURLs := make(map[string]bool)

	for _, pattern := range patterns {
		linkRegex := regexp.MustCompile(pattern)
		matches := linkRegex.FindAllStringSubmatch(html, 25)

		for _, match := range matches {
			if len(match) > 2 {
				rawURL := match[1]
				title := strings.TrimSpace(match[2])

				// Decode URL if it's encoded
				if strings.HasPrefix(rawURL, "http") {
					decodedURL, err := url.QueryUnescape(rawURL)
					if err == nil {
						rawURL = decodedURL
					}
				}

				// Skip invalid URLs and internal links
				if !strings.HasPrefix(rawURL, "http") ||
					strings.Contains(rawURL, "duckduckgo.com") ||
					strings.Contains(rawURL, "google.com") ||
					strings.Contains(rawURL, "bing.com") ||
					foundURLs[rawURL] {
					continue
				}

				if c.isValidWebsite(rawURL) && len(title) > 3 {
					foundURLs[rawURL] = true
					priority := 0.95 - float64(len(seeds))*0.05
					if priority < 0.1 {
						priority = 0.1
					}

					seeds = append(seeds, SeedURL{
						URL:         rawURL,
						Priority:    priority,
						Topic:       "web_result",
						Source:      "duckduckgo",
						Reliability: 0.85,
					})

					log.Printf("Found website from DDG: %s (title: %s)", rawURL, title)

					if len(seeds) >= 12 {
						break
					}
				}
			}
		}

		if len(seeds) >= 12 {
			break
		}
	}

	log.Printf("Total found %d real website URLs from DuckDuckGo for query: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) extractURLsFromStartpage(query string) []SeedURL {
	var seeds []SeedURL

	// Startpage is a privacy-focused search engine that proxies Google results
	searchURL := fmt.Sprintf("https://www.startpage.com/sp/search?query=%s&cat=web&language=english", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		log.Printf("Error creating Startpage request: %v", err)
		return seeds
	}

	// Add realistic browser headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://www.startpage.com/")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Error fetching Startpage results: %v", err)
		return seeds
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Non-200 status from Startpage: %d", resp.StatusCode)
		return seeds
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading Startpage response: %v", err)
		return seeds
	}

	html := string(body)
	log.Printf("Startpage response length: %d chars", len(html))

	// Multiple patterns to extract URLs from Startpage
	patterns := []string{
		// Main result URLs
		`data-target="([^"]+)"[^>]*class="[^"]*w-gl__result-title[^"]*"`,
		// Alternative patterns
		`<a[^>]*href="([^"]+)"[^>]*class="[^"]*result-link[^"]*"`,
		`<a[^>]*data-target="([^"]+)"[^>]*>`,
		// Direct external links
		`<a[^>]*href="(https?://[^"]+)"[^>]*class="[^"]*title[^"]*"`,
	}

	foundURLs := make(map[string]bool)

	for _, pattern := range patterns {
		linkRegex := regexp.MustCompile(pattern)
		matches := linkRegex.FindAllStringSubmatch(html, 20)

		for _, match := range matches {
			if len(match) > 1 {
				resultURL := match[1]

				// Decode URL if needed
				if decodedURL, err := url.QueryUnescape(resultURL); err == nil {
					resultURL = decodedURL
				}

				if !foundURLs[resultURL] && c.isValidWebsite(resultURL) {
					foundURLs[resultURL] = true
					priority := 0.9 - float64(len(seeds))*0.05
					if priority < 0.1 {
						priority = 0.1
					}

					seeds = append(seeds, SeedURL{
						URL:         resultURL,
						Priority:    priority,
						Topic:       "web_result",
						Source:      "startpage",
						Reliability: 0.9,
					})

					log.Printf("Found website from Startpage: %s", resultURL)

					if len(seeds) >= 10 {
						break
					}
				}
			}
		}

		if len(seeds) >= 10 {
			break
		}
	}

	log.Printf("Found %d website URLs from Startpage for query: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) extractURLsFromSearx(query string) []SeedURL {
	var seeds []SeedURL

	// Try multiple public Searx instances
	searxInstances := []string{
		"https://searx.be",
		"https://search.snopyta.org",
		"https://searx.info",
	}

	for _, instance := range searxInstances {
		searchURL := fmt.Sprintf("%s/search?q=%s&format=json", instance, url.QueryEscape(query))

		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json,text/plain,*/*")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error fetching from Searx instance %s: %v", instance, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		html := string(body)

		// Extract URLs using regex since we're dealing with HTML fallback
		linkRegex := regexp.MustCompile(`<a[^>]*href="(https?://[^"]+)"[^>]*>([^<]+)</a>`)
		matches := linkRegex.FindAllStringSubmatch(html, 15)

		for _, match := range matches {
			if len(match) > 2 {
				resultURL := match[1]
				title := strings.TrimSpace(match[2])

				if c.isValidWebsite(resultURL) && len(title) > 3 {
					priority := 0.8 - float64(len(seeds))*0.05
					if priority < 0.1 {
						priority = 0.1
					}

					seeds = append(seeds, SeedURL{
						URL:         resultURL,
						Priority:    priority,
						Topic:       "web_result",
						Source:      "searx",
						Reliability: 0.75,
					})

					log.Printf("Found website from Searx: %s", resultURL)

					if len(seeds) >= 8 {
						break
					}
				}
			}
		}

		if len(seeds) > 0 {
			break
		}
	}

	log.Printf("Found %d URLs from Searx for query: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) extractURLsFromBing(query string) []SeedURL {
	var seeds []SeedURL

	// Use Bing's search endpoint
	searchURL := fmt.Sprintf("https://www.bing.com/search?q=%s&count=20", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		log.Printf("Error creating Bing request: %v", err)
		return seeds
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Error fetching Bing results: %v", err)
		return seeds
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Non-200 status from Bing: %d", resp.StatusCode)
		return seeds
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading Bing response: %v", err)
		return seeds
	}

	html := string(body)

	// Extract result URLs from Bing
	patterns := []string{
		`<h2><a[^>]*href="([^"]+)"[^>]*>([^<]+)</a></h2>`,
		`<a[^>]*href="([^"]+)"[^>]*class="[^"]*titlelink[^"]*"[^>]*>([^<]+)</a>`,
		`<a[^>]*data-url="([^"]+)"[^>]*>([^<]+)</a>`,
	}

	foundURLs := make(map[string]bool)

	for _, pattern := range patterns {
		linkRegex := regexp.MustCompile(pattern)
		matches := linkRegex.FindAllStringSubmatch(html, 15)

		for _, match := range matches {
			if len(match) > 2 {
				resultURL := match[1]
				title := strings.TrimSpace(match[2])

				// Skip Bing internal URLs
				if strings.Contains(resultURL, "bing.com") ||
					strings.Contains(resultURL, "microsoft.com") ||
					foundURLs[resultURL] {
					continue
				}

				if c.isValidWebsite(resultURL) && len(title) > 3 {
					foundURLs[resultURL] = true
					priority := 0.8 - float64(len(seeds))*0.05
					if priority < 0.1 {
						priority = 0.1
					}

					seeds = append(seeds, SeedURL{
						URL:         resultURL,
						Priority:    priority,
						Topic:       "web_result",
						Source:      "bing",
						Reliability: 0.8,
					})

					log.Printf("Found website from Bing: %s", resultURL)

					if len(seeds) >= 10 {
						break
					}
				}
			}
		}

		if len(seeds) >= 10 {
			break
		}
	}

	log.Printf("Found %d URLs from Bing for query: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) getAcademicSources(query string) []SeedURL {
	var seeds []SeedURL
	queryLower := strings.ToLower(query)

	// Add academic and educational sources based on query characteristics
	if c.isAcademicQuery(queryLower) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://scholar.google.com/scholar?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "academic", Source: "google_scholar", Reliability: 0.9},
			{URL: fmt.Sprintf("https://www.researchgate.net/search?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "research", Source: "researchgate", Reliability: 0.85},
			{URL: fmt.Sprintf("https://arxiv.org/search/?query=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "preprint", Source: "arxiv", Reliability: 0.9},
			{URL: fmt.Sprintf("https://pubmed.ncbi.nlm.nih.gov/?term=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "medical", Source: "pubmed", Reliability: 0.95},
		}...)
	}

	// Add educational resources
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://www.coursera.org/search?query=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "education", Source: "coursera", Reliability: 0.8},
		{URL: fmt.Sprintf("https://www.khanacademy.org/search?page_search_query=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "education", Source: "khanacademy", Reliability: 0.85},
		{URL: fmt.Sprintf("https://www.edx.org/search?q=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "education", Source: "edx", Reliability: 0.8},
	}...)

	// Add technical documentation sources
	if c.isTechnicalQuery(queryLower) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://stackoverflow.com/search?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "technical", Source: "stackoverflow", Reliability: 0.85},
			{URL: fmt.Sprintf("https://github.com/search?q=%s&type=repositories", url.QueryEscape(query)), Priority: 0.6, Topic: "code", Source: "github", Reliability: 0.8},
			{URL: fmt.Sprintf("https://developer.mozilla.org/en-US/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "documentation", Source: "mdn", Reliability: 0.9},
		}...)
	}

	log.Printf("Added %d academic/educational seeds", len(seeds))
	return seeds
}

func (c *ContentCrawler) isAcademicQuery(query string) bool {
	academicKeywords := []string{
		"research", "study", "analysis", "theory", "hypothesis", "experiment",
		"paper", "journal", "publication", "academic", "scholar", "university",
		"thesis", "dissertation", "methodology", "literature review", "citation",
		"peer review", "scientific", "evidence", "data", "statistics",
	}

	for _, keyword := range academicKeywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}
	return false
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
		`(?i)<div[^>]*class="[^"]*mw-parser-output[^"]*"[^>]*>(.*?)</div>`,
		`(?i)<div[^>]*class="[^"]*mw-content-text[^"]*"[^>]*>(.*?)</div>`,
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

	// Filter out JavaScript requirement pages
	if strings.Contains(strings.ToLower(doc.Content), "javascript") && strings.Contains(strings.ToLower(doc.Content), "enable") {
		return false
	}

	// Filter out GitHub profile pages and repositories that are just personal profiles
	if strings.Contains(doc.URL, "github.com") {
		titleLower := strings.ToLower(doc.Title)
		contentLower := strings.ToLower(doc.Content)

		// Skip if it's just a personal profile without meaningful content
		if strings.Contains(titleLower, "github") &&
			(strings.Contains(contentLower, "repositories 0") ||
				strings.Contains(contentLower, "dismiss alert") ||
				strings.Contains(contentLower, "i love")) {
			return false
		}
	}

	// Filter out low-quality or irrelevant content
	lowQualityIndicators := []string{
		"dismiss alert", "repositories 0", "catboy", "instagram",
		"tiktok", "@ooo", "coming soon", "under construction",
		"lorem ipsum", "test page", "placeholder", "example",
	}
	contentLower := strings.ToLower(doc.Content)
	titleLower := strings.ToLower(doc.Title)

	for _, indicator := range lowQualityIndicators {
		if strings.Contains(contentLower, indicator) || strings.Contains(titleLower, indicator) {
			return false
		}
	}

	// Check for spam indicators
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

func (c *ContentCrawler) sortSeedsByPriority(seeds []SeedURL) {
	sort.Slice(seeds, func(i, j int) bool {
		return seeds[i].Priority > seeds[j].Priority
	})
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
				words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
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

func (c *ContentCrawler) extractSimpleTextContent(html string) string {
	// Remove scripts, styles, and other non-content tags
	content := html
	content = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<nav[^>]*>.*?</nav>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<header[^>]*>.*?</header>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<footer[^>]*>.*?</footer>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?i)<!--.*?-->`).ReplaceAllString(content, "")

	// Remove all HTML tags
	content = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(content, " ")

	// Decode HTML entities
	content = regexp.MustCompile(`&amp;`).ReplaceAllString(content, "&")
	content = regexp.MustCompile(`&lt;`).ReplaceAllString(content, "<")
	content = regexp.MustCompile(`&gt;`).ReplaceAllString(content, ">")
	content = regexp.MustCompile(`&quot;`).ReplaceAllString(content, "\"")
	content = regexp.MustCompile(`&#39;`).ReplaceAllString(content, "'")
	content = regexp.MustCompile(`&nbsp;`).ReplaceAllString(content, " ")

	// Clean up whitespace
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
	content = strings.TrimSpace(content)

	// Split into sentences and filter
	sentences := regexp.MustCompile(`[.!?]+`).Split(content, -1)
	var validSentences []string

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) > 20 && len(sentence) < 500 {
			// Skip sentences that look like navigation or boilerplate
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
	if len(result) > 5000 {
		result = result[:5000] + "..."
	}

	return result
}

func (c *ContentCrawler) getContentAggregatorSeeds(query string) []SeedURL {
	var seeds []SeedURL
	queryLower := strings.ToLower(query)

	// Add diverse content sources based on query type
	if strings.Contains(queryLower, "recipe") || strings.Contains(queryLower, "food") || strings.Contains(queryLower, "cooking") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.allrecipes.com/search/results/?search=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "recipe", Source: "allrecipes", Reliability: 0.8},
			{URL: fmt.Sprintf("https://www.foodnetwork.com/search/%s", url.QueryEscape(query)), Priority: 0.7, Topic: "recipe", Source: "foodnetwork", Reliability: 0.8},
			{URL: fmt.Sprintf("https://www.epicurious.com/search?content=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "recipe", Source: "epicurious", Reliability: 0.75},
		}...)
	}

	if strings.Contains(queryLower, "how to") || strings.Contains(queryLower, "tutorial") || strings.Contains(queryLower, "guide") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.wikihow.com/wikiHowTo?search=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "howto", Source: "wikihow", Reliability: 0.7},
			{URL: fmt.Sprintf("https://www.instructables.com/circuits/howto/%s/", url.QueryEscape(query)), Priority: 0.6, Topic: "diy", Source: "instructables", Reliability: 0.7},
		}...)
	}

	if strings.Contains(queryLower, "product") || strings.Contains(queryLower, "review") || strings.Contains(queryLower, "buy") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.amazon.com/s?k=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "product", Source: "amazon", Reliability: 0.7},
			{URL: fmt.Sprintf("https://www.newegg.com/p/pl?d=%s", url.QueryEscape(query)), Priority: 0.5, Topic: "product", Source: "newegg", Reliability: 0.65},
		}...)
	}

	// News and current events
	if strings.Contains(queryLower, "news") || strings.Contains(queryLower, "breaking") || strings.Contains(queryLower, "latest") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.reuters.com/search/news?blob=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "news", Source: "reuters", Reliability: 0.9},
			{URL: fmt.Sprintf("https://www.bbc.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "news", Source: "bbc", Reliability: 0.9},
			{URL: fmt.Sprintf("https://apnews.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "news", Source: "ap", Reliability: 0.95},
		}...)
	}

	// Health and medical
	if strings.Contains(queryLower, "health") || strings.Contains(queryLower, "medical") || strings.Contains(queryLower, "symptoms") || strings.Contains(queryLower, "disease") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.mayoclinic.org/search/search-results?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "health", Source: "mayo", Reliability: 0.95},
			{URL: fmt.Sprintf("https://www.webmd.com/search/search_results/default.aspx?query=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "health", Source: "webmd", Reliability: 0.8},
			{URL: fmt.Sprintf("https://medlineplus.gov/medlineplus-search.html?query=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "health", Source: "medlineplus", Reliability: 0.9},
		}...)
	}

	// Travel and places
	if strings.Contains(queryLower, "travel") || strings.Contains(queryLower, "visit") || strings.Contains(queryLower, "destination") || strings.Contains(queryLower, "trip") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.lonelyplanet.com/search?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "travel", Source: "lonelyplanet", Reliability: 0.8},
			{URL: fmt.Sprintf("https://www.tripadvisor.com/Search?q=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "travel", Source: "tripadvisor", Reliability: 0.75},
		}...)
	}

	// Always add some diverse general sources
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://medium.com/search?q=%s", url.QueryEscape(query)), Priority: 0.6, Topic: "blog", Source: "medium", Reliability: 0.7},
		{URL: fmt.Sprintf("https://www.quora.com/search?q=%s", url.QueryEscape(query)), Priority: 0.5, Topic: "qa", Source: "quora", Reliability: 0.6},
		{URL: fmt.Sprintf("https://www.reddit.com/search?q=%s", url.QueryEscape(query)), Priority: 0.5, Topic: "discussion", Source: "reddit", Reliability: 0.6},
		{URL: fmt.Sprintf("https://www.goodreads.com/search?q=%s", url.QueryEscape(query)), Priority: 0.5, Topic: "books", Source: "goodreads", Reliability: 0.7},
		{URL: fmt.Sprintf("https://www.imdb.com/find?q=%s", url.QueryEscape(query)), Priority: 0.5, Topic: "entertainment", Source: "imdb", Reliability: 0.8},
	}...)

	return seeds
}

func (c *ContentCrawler) getDirectSources(query string) []SeedURL {
	var seeds []SeedURL
	queryLower := strings.ToLower(query)

	log.Printf("Adding direct sources for query: %s", query)

	// Add high-quality educational and informational sources
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://www.nationalgeographic.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "educational", Source: "natgeo", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.smithsonianmag.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "educational", Source: "smithsonian", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.scientificamerican.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "science", Source: "sciam", Reliability: 0.9},
	}...)

	// Animal-related queries
	if strings.Contains(queryLower, "animal") || strings.Contains(queryLower, "bird") ||
		strings.Contains(queryLower, "duck") || strings.Contains(queryLower, "wildlife") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.allaboutbirds.org/guide/search?q=%s", url.QueryEscape(query)), Priority: 0.9, Topic: "wildlife", Source: "allaboutbirds", Reliability: 0.95},
			{URL: fmt.Sprintf("https://animaldiversity.org/search/?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "biology", Source: "animaldiversity", Reliability: 0.9},
		}...)
	}

	// History and culture
	if strings.Contains(queryLower, "history") || strings.Contains(queryLower, "culture") ||
		strings.Contains(queryLower, "ancient") || strings.Contains(queryLower, "civilization") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.history.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "history", Source: "history", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.worldhistory.org/search/?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "history", Source: "worldhistory", Reliability: 0.9},
		}...)
	}

	// Technology and science
	if c.isTechnicalQuery(queryLower) {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://techcrunch.com/?s=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "technology", Source: "techcrunch", Reliability: 0.8},
			{URL: fmt.Sprintf("https://arstechnica.com/search/?query=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "technology", Source: "arstechnica", Reliability: 0.85},
		}...)
	}

	// Health and medical
	if strings.Contains(queryLower, "health") || strings.Contains(queryLower, "medical") ||
		strings.Contains(queryLower, "disease") || strings.Contains(queryLower, "symptoms") {
		seeds = append(seeds, []SeedURL{
			{URL: fmt.Sprintf("https://www.healthline.com/search?q1=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "health", Source: "healthline", Reliability: 0.85},
			{URL: fmt.Sprintf("https://www.medicalnewstoday.com/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "health", Source: "medicalnewstoday", Reliability: 0.85},
		}...)
	}

	// Add some general high-quality sources
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://www.theatlantic.com/search/?q=%s", url.QueryEscape(query)), Priority: 0.7, Topic: "journalism", Source: "atlantic", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.newyorker.com/search/q/%s", url.QueryEscape(query)), Priority: 0.7, Topic: "journalism", Source: "newyorker", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.npr.org/search?query=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "news", Source: "npr", Reliability: 0.9},
		{URL: fmt.Sprintf("https://www.bbc.co.uk/search?q=%s", url.QueryEscape(query)), Priority: 0.8, Topic: "news", Source: "bbc", Reliability: 0.9},
	}...)

	log.Printf("Added %d direct source seeds", len(seeds))
	return seeds
}

func (c *ContentCrawler) isValidWebsite(urlStr string) bool {
	if urlStr == "" {
		return false
	}

	// Must be HTTP(S)
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return false
	}

	// Skip search engines and internal pages
	skipDomains := []string{
		"google.com", "bing.com", "yahoo.com", "duckduckgo.com", "startpage.com",
		"facebook.com", "twitter.com", "instagram.com", "linkedin.com",
		"ads.", "doubleclick.", "googletagmanager.com", "analytics.google.com",
		"googlesyndication.com", "googleadservices.com", "gstatic.com",
		"amazon-adsystem.com", "googletagservices.com", "outbrain.com",
		"taboola.com", "addthis.com", "sharethis.com", "disqus.com",
	}

	for _, domain := range skipDomains {
		if strings.Contains(urlStr, domain) {
			return false
		}
	}

	// Skip file downloads
	skipExtensions := []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".zip", ".rar", ".exe", ".dmg", ".mp3", ".mp4", ".avi", ".mov", ".jpg", ".jpeg",
		".png", ".gif", ".svg", ".ico", ".css", ".js", ".xml", ".json"}

	for _, ext := range skipExtensions {
		if strings.HasSuffix(strings.ToLower(urlStr), ext) {
			return false
		}
	}

	// Skip obvious spam patterns
	urlLower := strings.ToLower(urlStr)
	spamPatterns := []string{
		"casino", "poker", "gambling", "viagra", "cialis", "pharmacy",
		"weight-loss", "bitcoin", "cryptocurrency", "get-rich",
		"adult", "xxx", "porn", "escort", "dating",
	}

	for _, pattern := range spamPatterns {
		if strings.Contains(urlLower, pattern) {
			return false
		}
	}

	return true
}

func (c *ContentCrawler) getEntitySpecificSources(query string) []SeedURL {
	var seeds []SeedURL
	queryLower := strings.ToLower(strings.TrimSpace(query))

	log.Printf("Checking for entity-specific sources for: %s", query)

	// Check if this looks like an organization, company, or brand name
	if c.isEntityQuery(queryLower) {
		log.Printf("Detected entity query: %s", query)

		// Try to find official domains and primary sources
		entitySeeds := c.getOfficialEntitySources(query)
		seeds = append(seeds, entitySeeds...)

		// Add news and press coverage
		newsSeeds := c.getEntityNewsSources(query)
		seeds = append(seeds, newsSeeds...)
	} else {
		// Check if this is a "What is X" or "Who is X" type query
		extractedEntity := c.extractEntityFromQuestion(queryLower)
		if extractedEntity != "" {
			log.Printf("Detected question about entity: %s", extractedEntity)

			// Treat the extracted entity as an entity query
			entitySeeds := c.getOfficialEntitySources(extractedEntity)
			seeds = append(seeds, entitySeeds...)

			newsSeeds := c.getEntityNewsSources(extractedEntity)
			seeds = append(seeds, newsSeeds...)
		}
	}

	return seeds
}

func (c *ContentCrawler) isEntityQuery(query string) bool {
	words := strings.Fields(query)

	// Simple heuristics for entity detection:
	// 1. Short queries (1-3 words)
	// 2. Not containing question words or generic terms

	if len(words) > 4 {
		return false // Too long to be a simple entity
	}

	// Check for question words or instructional terms
	questionWords := []string{"what", "how", "why", "when", "where", "who", "which",
		"tutorial", "guide", "learn", "course", "training", "explain"}

	for _, word := range words {
		wordLower := strings.ToLower(word)
		for _, qw := range questionWords {
			if strings.Contains(wordLower, qw) {
				return false
			}
		}
	}

	// If it's 1-3 words and doesn't contain question terms, likely an entity
	return len(words) >= 1 && len(words) <= 3
}

func (c *ContentCrawler) getOfficialEntitySources(query string) []SeedURL {
	var seeds []SeedURL

	// Clean up the query for domain guessing
	cleanQuery := strings.ToLower(query)
	cleanQuery = regexp.MustCompile(`[^a-z0-9\s]`).ReplaceAllString(cleanQuery, "")
	cleanQuery = strings.ReplaceAll(cleanQuery, " ", "")

	// Try common official domain patterns
	officialPatterns := []string{
		"https://www.%s.com",
		"https://%s.com",
		"https://www.%s.org",
		"https://%s.org",
		"https://www.%s.io",
		"https://%s.io",
		"https://www.%s.net",
		"https://%s.net",
	}

	for i, pattern := range officialPatterns {
		officialURL := fmt.Sprintf(pattern, cleanQuery)
		priority := 0.95 - float64(i)*0.05 // Prioritize .com, then .org, etc.

		seeds = append(seeds, SeedURL{
			URL:         officialURL,
			Priority:    priority,
			Topic:       "official",
			Source:      "official_domain",
			Reliability: 0.95,
		})
	}

	// Add GitHub organization page
	seeds = append(seeds, SeedURL{
		URL:         fmt.Sprintf("https://github.com/%s", cleanQuery),
		Priority:    0.9,
		Topic:       "official",
		Source:      "github",
		Reliability: 0.9,
	})

	// Add social media profiles
	seeds = append(seeds, []SeedURL{
		{URL: fmt.Sprintf("https://twitter.com/%s", cleanQuery), Priority: 0.8, Topic: "social", Source: "twitter", Reliability: 0.8},
		{URL: fmt.Sprintf("https://linkedin.com/company/%s", cleanQuery), Priority: 0.8, Topic: "social", Source: "linkedin", Reliability: 0.8},
	}...)

	log.Printf("Generated %d official entity sources for: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) getEntityNewsSources(query string) []SeedURL {
	var seeds []SeedURL

	// High-quality news sources that cover tech/organizations
	newsPattern := "%s site:%s"
	newsSources := []struct {
		domain      string
		priority    float64
		reliability float64
	}{
		{"techcrunch.com", 0.9, 0.9},
		{"venturebeat.com", 0.85, 0.85},
		{"wired.com", 0.9, 0.9},
		{"arstechnica.com", 0.85, 0.9},
		{"theverge.com", 0.85, 0.85},
		{"reuters.com", 0.95, 0.95},
		{"bloomberg.com", 0.9, 0.9},
		{"forbes.com", 0.8, 0.8},
	}

	for _, source := range newsSources {
		// Use site-specific search
		searchQuery := fmt.Sprintf(newsPattern, url.QueryEscape(query), source.domain)
		seeds = append(seeds, SeedURL{
			URL:         fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(searchQuery)),
			Priority:    source.priority * 0.8, // Slightly lower than official sources
			Topic:       "news",
			Source:      source.domain,
			Reliability: source.reliability,
		})
	}

	log.Printf("Generated %d news sources for entity: %s", len(seeds), query)
	return seeds
}

func (c *ContentCrawler) extractEntityFromQuestion(query string) string {
	// Common question patterns that indicate entity queries
	patterns := []struct {
		prefix   string
		suffixes []string
	}{
		{"what is ", []string{"", " about", " like", " exactly"}},
		{"who is ", []string{"", " exactly", " really"}},
		{"what are ", []string{"", " about", " like"}},
		{"tell me about ", []string{""}},
		{"information about ", []string{""}},
		{"about ", []string{""}},
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))

	// Try to extract entity from question patterns
	for _, pattern := range patterns {
		if strings.HasPrefix(queryLower, pattern.prefix) {
			// Remove the question prefix
			entity := strings.TrimPrefix(queryLower, pattern.prefix)

			// Remove common suffixes
			for _, suffix := range pattern.suffixes {
				if suffix != "" && strings.HasSuffix(entity, suffix) {
					entity = strings.TrimSuffix(entity, suffix)
				}
			}

			// Clean up the entity
			entity = strings.TrimSpace(entity)

			// Remove common articles from the beginning
			articles := []string{"a ", "an ", "the "}
			for _, article := range articles {
				if strings.HasPrefix(entity, article) {
					entity = strings.TrimPrefix(entity, article)
					entity = strings.TrimSpace(entity)
					break
				}
			}

			// Check if the remaining entity looks valid
			if c.isValidEntity(entity) {
				log.Printf("Extracted entity '%s' from question: %s", entity, query)
				return entity
			}
		}
	}

	return ""
}

func (c *ContentCrawler) isValidEntity(entity string) bool {
	// Basic validation for extracted entities
	words := strings.Fields(entity)

	// Should be 1-4 words
	if len(words) == 0 || len(words) > 4 {
		return false
	}

	// Should not contain remaining question words
	remainingQuestionWords := []string{"how", "why", "when", "where", "which", "can", "do", "does", "did"}
	for _, word := range words {
		for _, qw := range remainingQuestionWords {
			if strings.Contains(word, qw) {
				return false
			}
		}
	}

	// Should have reasonable length
	if len(entity) < 2 || len(entity) > 50 {
		return false
	}

	return true
}

func (c *ContentCrawler) getGeneralKnowledgeSources(topic string) []SeedURL {
	var seeds []SeedURL

	// Educational and reference sources for general knowledge
	sources := []struct {
		urlTemplate string
		priority    float64
		source      string
		reliability float64
	}{
		{"https://www.nationalgeographic.com/search?q=%s", 0.6, "natgeo", 0.8},
		{"https://kids.nationalgeographic.com/search?q=%s", 0.5, "natgeo_kids", 0.75},
		{"https://www.howstuffworks.com/search?terms=%s", 0.4, "howstuffworks", 0.7},
		{"https://www.smithsonianmag.com/search/?q=%s", 0.3, "smithsonian", 0.8},
	}

	for _, source := range sources {
		url := fmt.Sprintf(source.urlTemplate, url.QueryEscape(topic))
		seeds = append(seeds, SeedURL{
			URL:         url,
			Priority:    source.priority,
			Topic:       "education",
			Source:      source.source,
			Reliability: source.reliability,
		})
	}

	return seeds
}

func (c *ContentCrawler) isGeneralKnowledgeQuery(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	words := strings.Fields(queryLower)

	// Single word queries are likely general knowledge (animals, concepts, etc.)
	if len(words) == 1 {
		word := words[0]
		// Skip technical terms or very short words
		if len(word) >= 3 && !c.isTechnicalTerm(word) {
			return true
		}
	}

	// Two-word queries that might be general knowledge
	if len(words) == 2 {
		commonPatterns := []string{
			"frog pictures", "duck images", "cat behavior", "dog breeds",
			"python language", "javascript tutorial", "web development",
		}
		for _, pattern := range commonPatterns {
			if strings.Contains(queryLower, pattern) {
				return true
			}
		}
	}

	return false
}

func (c *ContentCrawler) isTechnicalTerm(word string) bool {
	technicalTerms := []string{
		"api", "json", "xml", "http", "https", "www", "css", "html",
		"sql", "url", "uri", "tcp", "udp", "ssh", "ftp", "git",
	}

	for _, term := range technicalTerms {
		if word == term {
			return true
		}
	}
	return false
}
