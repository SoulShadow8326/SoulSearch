package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Page struct {
	URL     string
	Title   string
	Content string
	Links   []string
	Hash    string
	Crawled time.Time
}

type Crawler struct {
	visited    map[string]bool
	queue      []string
	maxPages   int
	pages      []Page
	mutex      sync.RWMutex
	client     *http.Client
	urlRegex   *regexp.Regexp
	titleRegex *regexp.Regexp
}

func NewCrawler(maxPages int) *Crawler {
	return &Crawler{
		visited:    make(map[string]bool),
		queue:      make([]string, 0),
		maxPages:   maxPages,
		pages:      make([]Page, 0),
		client:     &http.Client{Timeout: 10 * time.Second},
		urlRegex:   regexp.MustCompile(`href="([^"]+)"`),
		titleRegex: regexp.MustCompile(`<title[^>]*>([^<]+)</title>`),
	}
}

func (c *Crawler) CrawlFromSeed(seedURL string) {
	c.queue = append(c.queue, seedURL)

	for len(c.queue) > 0 && len(c.pages) < c.maxPages {
		currentURL := c.queue[0]
		c.queue = c.queue[1:]

		if c.visited[currentURL] {
			continue
		}

		page := c.crawlPage(currentURL)
		if page != nil {
			c.mutex.Lock()
			c.pages = append(c.pages, *page)
			c.visited[currentURL] = true
			c.mutex.Unlock()

			for _, link := range page.Links {
				if !c.visited[link] && c.isValidURL(link) {
					c.queue = append(c.queue, link)
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	c.savePages()
}

func (c *Crawler) crawlPage(pageURL string) *Page {
	resp, err := c.client.Get(pageURL)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	contentStr := string(content)
	title := c.extractTitle(contentStr)
	textContent := c.extractText(contentStr)
	links := c.extractLinks(contentStr, pageURL)

	hash := fmt.Sprintf("%x", md5.Sum([]byte(textContent)))

	return &Page{
		URL:     pageURL,
		Title:   title,
		Content: textContent,
		Links:   links,
		Hash:    hash,
		Crawled: time.Now(),
	}
}

func (c *Crawler) extractTitle(html string) string {
	matches := c.titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func (c *Crawler) extractText(html string) string {
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	scriptRegex := regexp.MustCompile(`<script[^>]*>.*?</script>`)
	styleRegex := regexp.MustCompile(`<style[^>]*>.*?</style>`)

	text := scriptRegex.ReplaceAllString(html, "")
	text = styleRegex.ReplaceAllString(text, "")
	text = tagRegex.ReplaceAllString(text, " ")

	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

func (c *Crawler) extractLinks(html, baseURL string) []string {
	links := make([]string, 0)
	matches := c.urlRegex.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(match) > 1 {
			link := match[1]
			absURL := c.resolveURL(link, baseURL)
			if absURL != "" {
				links = append(links, absURL)
			}
		}
	}

	return links
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

	return parsedURL.Scheme == "http" || parsedURL.Scheme == "https"
}

func (c *Crawler) savePages() {
	os.MkdirAll("data", 0755)

	file, err := os.Create("data/pages.dat")
	if err != nil {
		return
	}
	defer file.Close()

	for _, page := range c.pages {
		data := fmt.Sprintf("%s|%s|%s|%s|%d\n",
			page.URL,
			page.Title,
			strings.ReplaceAll(page.Content, "\n", " "),
			page.Hash,
			page.Crawled.Unix())
		file.WriteString(data)
	}
}
