package main

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Page struct {
	URL       string
	Title     string
	Content   string
	Links     []string
	Hash      string
	Crawled   time.Time
	Depth     int
	Quality   float64
	Size      int
	Domain    string
	Language  string
	Redirects int
}

type URLInfo struct {
	URL      string
	Depth    int
	Priority float64
	Source   string
	Retries  int
}

type CrawlStats struct {
	TotalRequests    int
	SuccessfulCrawls int
	FailedCrawls     int
	DuplicatePages   int
	BlockedPages     int
	LowQualityPages  int
	TotalSize        int64
	AverageRespTime  time.Duration
}

type Crawler struct {
	visited      map[string]bool
	queue        []*URLInfo
	maxPages     int
	maxDepth     int
	pages        []Page
	mutex        sync.RWMutex
	client       *http.Client
	robotsCache  map[string]bool
	domainDelays map[string]time.Time
	stats        CrawlStats
	contentTypes map[string]bool
	excludeRegex []*regexp.Regexp
	includeRegex []*regexp.Regexp
	workers      int
	workerChan   chan *URLInfo
	resultChan   chan *Page
	stopChan     chan bool
	wg           sync.WaitGroup
}

func NewCrawler(maxPages int) *Crawler {
	contentTypes := map[string]bool{
		"text/html":             true,
		"application/xhtml+xml": true,
	}

	excludePatterns := []string{
		`\.(pdf|doc|docx|xls|xlsx|ppt|pptx|zip|rar|exe|dmg|iso)$`,
		`\.(jpg|jpeg|png|gif|bmp|svg|ico|webp)$`,
		`\.(mp3|mp4|avi|mov|wmv|flv|webm|mkv)$`,
		`\.(css|js|json|xml|txt)$`,
		`/admin/|/wp-admin/|/login|/register`,
		`\?.*print=|/print/|\.print$`,
		`/tag/|/tags/|/category/|/categories/`,
		`\.(gz|tar|7z|bz2)$`,
	}

	includePatterns := []string{
		`^https?://[^/]+/$`,
		`^https?://[^/]+/[^?]*$`,
		`/(article|post|blog|news|story|content)/`,
		`/\d{4}/\d{2}/`,
		`/(about|contact|services|products|help|faq|guide)/`,
	}

	var excludeRegex []*regexp.Regexp
	for _, pattern := range excludePatterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			excludeRegex = append(excludeRegex, regex)
		}
	}

	var includeRegex []*regexp.Regexp
	for _, pattern := range includePatterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			includeRegex = append(includeRegex, regex)
		}
	}

	workers := 5

	return &Crawler{
		visited:      make(map[string]bool),
		queue:        make([]*URLInfo, 0),
		maxPages:     maxPages,
		maxDepth:     5,
		pages:        make([]Page, 0),
		client:       &http.Client{Timeout: 30 * time.Second},
		robotsCache:  make(map[string]bool),
		domainDelays: make(map[string]time.Time),
		contentTypes: contentTypes,
		excludeRegex: excludeRegex,
		includeRegex: includeRegex,
		workers:      workers,
		workerChan:   make(chan *URLInfo, workers*2),
		resultChan:   make(chan *Page, workers*2),
		stopChan:     make(chan bool),
	}
}

func (c *Crawler) CrawlFromSeed(seedURL string) {
	c.addToQueue(seedURL, 0, 1.0, "seed")

	for i := 0; i < c.workers; i++ {
		c.wg.Add(1)
		go c.worker()
	}

	go c.coordinator()

	c.wg.Wait()
	close(c.resultChan)

	for page := range c.resultChan {
		if page != nil {
			c.mutex.Lock()
			c.pages = append(c.pages, *page)
			c.mutex.Unlock()
		}
	}

	c.savePages()
	c.printStats()
}

func (c *Crawler) addToQueue(url string, depth int, priority float64, source string) {
	if c.shouldSkipURL(url) || depth > c.maxDepth {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.visited[url] {
		return
	}

	urlInfo := &URLInfo{
		URL:      url,
		Depth:    depth,
		Priority: priority,
		Source:   source,
		Retries:  0,
	}

	c.queue = append(c.queue, urlInfo)
	c.sortQueue()
}

func (c *Crawler) sortQueue() {
	sort.Slice(c.queue, func(i, j int) bool {
		if c.queue[i].Depth != c.queue[j].Depth {
			return c.queue[i].Depth < c.queue[j].Depth
		}
		return c.queue[i].Priority > c.queue[j].Priority
	})
}

func (c *Crawler) coordinator() {
	defer close(c.workerChan)

	for len(c.pages) < c.maxPages {
		c.mutex.Lock()
		if len(c.queue) == 0 {
			c.mutex.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}

		urlInfo := c.queue[0]
		c.queue = c.queue[1:]

		if c.visited[urlInfo.URL] {
			c.mutex.Unlock()
			continue
		}

		c.visited[urlInfo.URL] = true
		c.mutex.Unlock()

		if c.respectRateLimit(urlInfo.URL) {
			select {
			case c.workerChan <- urlInfo:
			case <-c.stopChan:
				return
			}
		}
	}
}

func (c *Crawler) worker() {
	defer c.wg.Done()

	for urlInfo := range c.workerChan {
		if len(c.pages) >= c.maxPages {
			return
		}

		page := c.crawlPage(urlInfo)
		if page != nil {
			c.resultChan <- page
			c.processPageLinks(page)
		}
	}
}

func (c *Crawler) respectRateLimit(pageURL string) bool {
	domain := c.extractDomain(pageURL)

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if lastCrawl, exists := c.domainDelays[domain]; exists {
		if time.Since(lastCrawl) < 1*time.Second {
			time.Sleep(1*time.Second - time.Since(lastCrawl))
		}
	}

	c.domainDelays[domain] = time.Now()
	return true
}

func (c *Crawler) crawlPage(urlInfo *URLInfo) *Page {
	start := time.Now()
	c.stats.TotalRequests++

	if !c.checkRobotsTxt(urlInfo.URL) {
		c.stats.BlockedPages++
		return nil
	}

	req, err := http.NewRequest("GET", urlInfo.URL, nil)
	if err != nil {
		c.stats.FailedCrawls++
		return nil
	}

	req.Header.Set("User-Agent", "SoulSearch-Bot/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		c.stats.FailedCrawls++
		if urlInfo.Retries < 2 {
			urlInfo.Retries++
			c.addToQueue(urlInfo.URL, urlInfo.Depth, urlInfo.Priority*0.8, "retry")
		}
		return nil
	}
	defer resp.Body.Close()

	if !c.isValidContentType(resp.Header.Get("Content-Type")) {
		c.stats.FailedCrawls++
		return nil
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		c.stats.FailedCrawls++
		return nil
	}

	contentStr := string(content)
	title := c.extractTitle(contentStr)
	textContent := c.extractText(contentStr)
	links := c.extractLinks(contentStr, urlInfo.URL)

	quality := c.assessContentQuality(title, textContent, urlInfo.URL)
	if quality < 0.3 {
		c.stats.LowQualityPages++
		return nil
	}

	hash := fmt.Sprintf("%x", md5.Sum([]byte(textContent)))

	if c.isDuplicate(hash) {
		c.stats.DuplicatePages++
		return nil
	}

	domain := c.extractDomain(urlInfo.URL)
	language := c.detectLanguage(textContent)

	c.stats.SuccessfulCrawls++
	c.stats.TotalSize += int64(len(content))
	c.stats.AverageRespTime = (c.stats.AverageRespTime + time.Since(start)) / 2

	return &Page{
		URL:       urlInfo.URL,
		Title:     title,
		Content:   textContent,
		Links:     links,
		Hash:      hash,
		Crawled:   time.Now(),
		Depth:     urlInfo.Depth,
		Quality:   quality,
		Size:      len(content),
		Domain:    domain,
		Language:  language,
		Redirects: 0,
	}
}

func (c *Crawler) extractTitle(html string) string {
	titleRegex := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.ReplaceAll(title, "\r", " ")
		title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
		return title
	}

	h1Regex := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	matches = h1Regex.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

func (c *Crawler) extractText(html string) string {
	scriptRegex := regexp.MustCompile(`<script[^>]*>.*?</script>`)
	styleRegex := regexp.MustCompile(`<style[^>]*>.*?</style>`)
	commentRegex := regexp.MustCompile(`<!--.*?-->`)
	navRegex := regexp.MustCompile(`<nav[^>]*>.*?</nav>`)
	footerRegex := regexp.MustCompile(`<footer[^>]*>.*?</footer>`)
	headerRegex := regexp.MustCompile(`<header[^>]*>.*?</header>`)
	asideRegex := regexp.MustCompile(`<aside[^>]*>.*?</aside>`)

	text := scriptRegex.ReplaceAllString(html, "")
	text = styleRegex.ReplaceAllString(text, "")
	text = commentRegex.ReplaceAllString(text, "")
	text = navRegex.ReplaceAllString(text, "")
	text = footerRegex.ReplaceAllString(text, "")
	text = headerRegex.ReplaceAllString(text, "")
	text = asideRegex.ReplaceAllString(text, "")

	tagRegex := regexp.MustCompile(`<[^>]*>`)
	text = tagRegex.ReplaceAllString(text, " ")

	entityRegex := regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
	text = entityRegex.ReplaceAllStringFunc(text, func(entity string) string {
		switch entity {
		case "&amp;":
			return "&"
		case "&lt;":
			return "<"
		case "&gt;":
			return ">"
		case "&quot;":
			return "\""
		case "&apos;":
			return "'"
		case "&nbsp;":
			return " "
		default:
			return " "
		}
	})

	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

func (c *Crawler) extractLinks(html, baseURL string) []string {
	links := make([]string, 0)
	urlRegex := regexp.MustCompile(`href\s*=\s*["']([^"']+)["']`)
	matches := urlRegex.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(match) > 1 {
			link := match[1]
			absURL := c.resolveURL(link, baseURL)
			if absURL != "" && c.isValidURL(absURL) {
				links = append(links, absURL)
			}
		}
	}

	return c.deduplicateLinks(links)
}

func (c *Crawler) deduplicateLinks(links []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(links))

	for _, link := range links {
		if !seen[link] {
			seen[link] = true
			result = append(result, link)
		}
	}

	return result
}

func (c *Crawler) shouldSkipURL(url string) bool {
	for _, regex := range c.excludeRegex {
		if regex.MatchString(url) {
			return true
		}
	}

	if len(c.includeRegex) > 0 {
		hasMatch := false
		for _, regex := range c.includeRegex {
			if regex.MatchString(url) {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			return true
		}
	}

	if len(url) > 2000 {
		return true
	}

	return false
}

func (c *Crawler) extractDomain(urlStr string) string {
	if parsedURL, err := url.Parse(urlStr); err == nil {
		return parsedURL.Hostname()
	}
	return ""
}

func (c *Crawler) checkRobotsTxt(urlStr string) bool {
	domain := c.extractDomain(urlStr)

	c.mutex.RLock()
	if allowed, exists := c.robotsCache[domain]; exists {
		c.mutex.RUnlock()
		return allowed
	}
	c.mutex.RUnlock()

	robotsURL := fmt.Sprintf("https://%s/robots.txt", domain)
	resp, err := c.client.Get(robotsURL)
	if err != nil {
		c.mutex.Lock()
		c.robotsCache[domain] = true
		c.mutex.Unlock()
		return true
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		c.mutex.Lock()
		c.robotsCache[domain] = true
		c.mutex.Unlock()
		return true
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		c.mutex.Lock()
		c.robotsCache[domain] = true
		c.mutex.Unlock()
		return true
	}

	allowed := c.parseRobotsTxt(string(content), urlStr)

	c.mutex.Lock()
	c.robotsCache[domain] = allowed
	c.mutex.Unlock()

	return allowed
}

func (c *Crawler) parseRobotsTxt(robotsContent, urlStr string) bool {
	lines := strings.Split(robotsContent, "\n")
	userAgentMatch := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(strings.ToLower(line), "user-agent:") {
			agent := strings.TrimSpace(line[11:])
			userAgentMatch = agent == "*" || strings.Contains(agent, "SoulSearch")
			continue
		}

		if userAgentMatch && strings.HasPrefix(strings.ToLower(line), "disallow:") {
			disallowPath := strings.TrimSpace(line[9:])
			if disallowPath == "/" {
				return false
			}
			if disallowPath != "" && strings.Contains(urlStr, disallowPath) {
				return false
			}
		}
	}

	return true
}

func (c *Crawler) isValidContentType(contentType string) bool {
	contentType = strings.ToLower(strings.Split(contentType, ";")[0])
	return c.contentTypes[contentType]
}

func (c *Crawler) assessContentQuality(title, content, url string) float64 {
	score := 0.0

	if len(title) > 10 && len(title) < 200 {
		score += 0.3
	}

	if len(content) > 200 && len(content) < 50000 {
		score += 0.3
	}

	words := strings.Fields(content)
	if len(words) > 50 {
		score += 0.2
	}

	sentences := strings.Split(content, ".")
	if len(sentences) > 5 {
		score += 0.1
	}

	if !strings.Contains(url, "404") && !strings.Contains(url, "error") {
		score += 0.1
	}

	return math.Min(score, 1.0)
}

func (c *Crawler) isDuplicate(hash string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	for _, page := range c.pages {
		if page.Hash == hash {
			return true
		}
	}
	return false
}

func (c *Crawler) detectLanguage(content string) string {
	englishWords := []string{"the", "and", "or", "but", "in", "on", "at", "to", "for", "of", "with", "by"}
	content = strings.ToLower(content)

	englishCount := 0
	words := strings.Fields(content)
	if len(words) == 0 {
		return "unknown"
	}

	for _, word := range englishWords {
		if strings.Contains(content, " "+word+" ") {
			englishCount++
		}
	}

	if float64(englishCount)/float64(len(englishWords)) > 0.3 {
		return "en"
	}

	return "unknown"
}

func (c *Crawler) processPageLinks(page *Page) {
	for _, link := range page.Links {
		priority := c.calculateLinkPriority(link, page)
		c.addToQueue(link, page.Depth+1, priority, page.URL)
	}
}

func (c *Crawler) calculateLinkPriority(link string, sourcePage *Page) float64 {
	priority := 0.5

	if c.extractDomain(link) == sourcePage.Domain {
		priority += 0.2
	}

	linkLower := strings.ToLower(link)
	if strings.Contains(linkLower, "article") || strings.Contains(linkLower, "post") ||
		strings.Contains(linkLower, "blog") || strings.Contains(linkLower, "news") {
		priority += 0.3
	}

	if strings.Contains(linkLower, "category") || strings.Contains(linkLower, "tag") {
		priority -= 0.2
	}

	if sourcePage.Quality > 0.7 {
		priority += 0.1
	}

	return math.Max(0.1, math.Min(1.0, priority))
}

func (c *Crawler) printStats() {
	log.Printf("Crawl Statistics:")
	log.Printf("  Total Requests: %d", c.stats.TotalRequests)
	log.Printf("  Successful Crawls: %d", c.stats.SuccessfulCrawls)
	log.Printf("  Failed Crawls: %d", c.stats.FailedCrawls)
	log.Printf("  Duplicate Pages: %d", c.stats.DuplicatePages)
	log.Printf("  Blocked Pages: %d", c.stats.BlockedPages)
	log.Printf("  Low Quality Pages: %d", c.stats.LowQualityPages)
	log.Printf("  Total Size: %d KB", c.stats.TotalSize/1024)
	log.Printf("  Average Response Time: %v", c.stats.AverageRespTime)
	log.Printf("  Success Rate: %.1f%%", float64(c.stats.SuccessfulCrawls)/float64(c.stats.TotalRequests)*100)
}

func (c *Crawler) resolveURL(href, base string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return ""
	}

	relURL, err := url.Parse(href)
	if err != nil {
		return ""
	}

	absURL := baseURL.ResolveReference(relURL)
	return absURL.String()
}

func (c *Crawler) isValidURL(urlStr string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	if parsedURL.Host == "" {
		return false
	}

	if strings.Contains(parsedURL.Fragment, "#") {
		return false
	}

	return true
}

func (c *Crawler) savePages() {
	err := os.MkdirAll("data", 0755)
	if err != nil {
		log.Printf("Error creating data directory: %v", err)
		return
	}

	file, err := os.Create("data/pages.dat")
	if err != nil {
		log.Printf("Error creating pages file: %v", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, page := range c.pages {
		data := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%.2f|%s|%s|%d\n",
			page.URL,
			strings.ReplaceAll(page.Title, "|", ""),
			strings.ReplaceAll(page.Content, "\n", " "),
			page.Hash,
			page.Crawled.Unix(),
			page.Depth,
			page.Quality,
			page.Domain,
			page.Language,
			page.Size)

		if _, err := writer.WriteString(data); err != nil {
			log.Printf("Error writing page data: %v", err)
		}
	}

	log.Printf("Saved %d pages to data/pages.dat", len(c.pages))
}
