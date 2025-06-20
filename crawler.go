package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type CrawlTask struct {
	URL       string    `json:"url"`
	Depth     int       `json:"depth"`
	Priority  float64   `json:"priority"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
}

type CrawlResult struct {
	URL        string            `json:"url"`
	Title      string            `json:"title"`
	Content    string            `json:"content"`
	Links      []string          `json:"links"`
	Keywords   []string          `json:"keywords"`
	Language   string            `json:"language"`
	Quality    float64           `json:"quality"`
	Success    bool              `json:"success"`
	Error      string            `json:"error,omitempty"`
	Duration   time.Duration     `json:"duration"`
	Size       int               `json:"size"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

type WorkerStats struct {
	ID             int           `json:"id"`
	TasksProcessed int64         `json:"tasks_processed"`
	TasksSuccess   int64         `json:"tasks_success"`
	TasksFailed    int64         `json:"tasks_failed"`
	AvgDuration    time.Duration `json:"avg_duration"`
	LastActive     time.Time     `json:"last_active"`
	Status         string        `json:"status"`
}

type CrawlerMaster struct {
	taskQueue    chan CrawlTask
	resultQueue  chan CrawlResult
	workers      []*CrawlWorker
	numWorkers   int
	visited      sync.Map
	urlQueue     sync.Map
	indexer      *Indexer
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	stats        sync.Map
	totalTasks   int64
	totalResults int64
	sockPath     string
	listener     net.Listener
	client       *http.Client
}

type CrawlWorker struct {
	id           int
	master       *CrawlerMaster
	client       *http.Client
	stats        *WorkerStats
	running      atomic.Bool
	taskCount    int64
	successCount int64
	failCount    int64
}

type IPCMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type CrawlerClientCmd struct {
	sockPath string
}

func CreateDistributedCrawler(numWorkers int, sockPath string) *CrawlerMaster {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU() * 4
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	master := &CrawlerMaster{
		taskQueue:   make(chan CrawlTask, numWorkers*10),
		resultQueue: make(chan CrawlResult, numWorkers*10),
		numWorkers:  numWorkers,
		indexer:     CreateIndexer(),
		ctx:         ctx,
		cancel:      cancel,
		sockPath:    sockPath,
		client:      client,
	}

	for i := 0; i < numWorkers; i++ {
		worker := &CrawlWorker{
			id:     i,
			master: master,
			client: client,
			stats: &WorkerStats{
				ID:         i,
				Status:     "idle",
				LastActive: time.Now(),
			},
		}
		master.workers = append(master.workers, worker)
		master.stats.Store(i, worker.stats)
	}

	return master
}

func (cm *CrawlerMaster) Start() error {
	os.Remove(cm.sockPath)

	var err error
	cm.listener, err = net.Listen("unix", cm.sockPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket: %v", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nShutting down distributed crawler...")
		cm.Stop()
		os.Remove(cm.sockPath)
		os.Exit(0)
	}()

	go cm.handleConnections()
	go cm.processResults()

	for _, worker := range cm.workers {
		cm.wg.Add(1)
		go worker.start()
	}

	go cm.monitorSystem()

	seedURLs := cm.getHighQualitySeeds()
	go cm.seedCrawler(seedURLs)

	fmt.Printf("Distributed crawler started with %d workers on %s\n", cm.numWorkers, cm.sockPath)
	return nil
}

func (cm *CrawlerMaster) Stop() {
	fmt.Println("Stopping crawler workers...")
	cm.cancel()
	close(cm.taskQueue)
	cm.wg.Wait()

	if cm.listener != nil {
		cm.listener.Close()
	}

	os.Remove(cm.sockPath)
	fmt.Println("Cleanup complete.")
}

func (cm *CrawlerMaster) handleConnections() {
	for {
		conn, err := cm.listener.Accept()
		if err != nil {
			select {
			case <-cm.ctx.Done():
				return
			default:
				continue
			}
		}
		go cm.handleConnection(conn)
	}
}

func (cm *CrawlerMaster) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg IPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "ADD_TASK":
			if taskData, ok := msg.Payload.(map[string]interface{}); ok {
				task := CrawlTask{
					URL:       getString(taskData, "url"),
					Depth:     getInt(taskData, "depth"),
					Priority:  getFloat(taskData, "priority"),
					Source:    getString(taskData, "source"),
					Timestamp: time.Now(),
				}
				cm.AddTask(task)
			}
		case "GET_STATS":
			stats := cm.GetStats()
			response, _ := json.Marshal(IPCMessage{
				Type:    "STATS",
				Payload: stats,
			})
			conn.Write(append(response, '\n'))
		case "BULK_ADD":
			if urls, ok := msg.Payload.([]interface{}); ok {
				for _, urlItem := range urls {
					if urlStr, ok := urlItem.(string); ok {
						task := CrawlTask{
							URL:       urlStr,
							Depth:     0,
							Priority:  1.0,
							Source:    "bulk",
							Timestamp: time.Now(),
						}
						cm.AddTask(task)
					}
				}
			}
		case "SEARCH":
			if searchData, ok := msg.Payload.(map[string]interface{}); ok {
				query := getString(searchData, "query")
				limit := getInt(searchData, "limit")
				if limit == 0 {
					limit = 10
				}

				// Search the shared index first
				results := cm.searchSharedIndex(query, limit)

				// If no results found, trigger real-time crawling for the query
				if len(results) == 0 {
					go cm.crawlForQuery(query, conn)
				} else {
					// Return existing results immediately
					response, _ := json.Marshal(IPCMessage{
						Type: "SEARCH_RESULTS",
						Payload: map[string]interface{}{
							"results": results,
							"query":   query,
							"total":   len(results),
						},
					})
					conn.Write(append(response, '\n'))
				}
			}
		}
	}
}

func (cm *CrawlerMaster) AddTask(task CrawlTask) {
	urlHash := hashURL(task.URL)
	if _, exists := cm.visited.LoadOrStore(urlHash, true); !exists {
		select {
		case cm.taskQueue <- task:
			atomic.AddInt64(&cm.totalTasks, 1)
		default:
		}
	}
}

func (cm *CrawlerMaster) processResults() {
	for {
		select {
		case result := <-cm.resultQueue:
			atomic.AddInt64(&cm.totalResults, 1)

			if result.Success {
				doc := Document{
					URL:      result.URL,
					Title:    result.Title,
					Content:  result.Content,
					Length:   result.Size,
					PageRank: result.Quality,
				}

				go cm.indexer.AddDocument(doc)

				sharedIndex := GetGlobalSharedIndex()
				sharedIndex.AddDocument(doc)

				for _, link := range result.Links {
					if cm.isValidURL(link) {
						task := CrawlTask{
							URL:       link,
							Depth:     0,
							Priority:  0.5,
							Source:    "discovered",
							Timestamp: time.Now(),
						}
						cm.AddTask(task)
					}
				}
			}
		case <-cm.ctx.Done():
			return
		}
	}
}

func (w *CrawlWorker) start() {
	defer w.master.wg.Done()
	w.running.Store(true)

	for {
		select {
		case task := <-w.master.taskQueue:
			w.processTask(task)
		case <-w.master.ctx.Done():
			w.running.Store(false)
			return
		}
	}
}

func (w *CrawlWorker) processTask(task CrawlTask) {
	start := time.Now()
	w.stats.Status = "working"
	w.stats.LastActive = start

	atomic.AddInt64(&w.taskCount, 1)

	result := w.crawlURL(task)
	result.Duration = time.Since(start)

	if result.Success {
		atomic.AddInt64(&w.successCount, 1)
	} else {
		atomic.AddInt64(&w.failCount, 1)
	}

	w.stats.TasksProcessed = atomic.LoadInt64(&w.taskCount)
	w.stats.TasksSuccess = atomic.LoadInt64(&w.successCount)
	w.stats.TasksFailed = atomic.LoadInt64(&w.failCount)
	w.stats.Status = "idle"

	select {
	case w.master.resultQueue <- result:
	default:
	}
}

func (w *CrawlWorker) crawlURL(task CrawlTask) CrawlResult {
	result := CrawlResult{
		URL:       task.URL,
		Success:   false,
		Timestamp: time.Now(),
	}

	req, err := http.NewRequestWithContext(w.master.ctx, "GET", task.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	req.Header.Set("User-Agent", "SoulSearch/1.0 (Enterprise Crawler)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := w.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}

	body := make([]byte, 2*1024*1024)
	n, _ := resp.Body.Read(body)
	body = body[:n]
	result.Size = n

	content := string(body)
	result.Title = extractTitle(content)
	result.Content = extractTextContent(content)
	result.Links = extractLinks(content, task.URL)
	result.Keywords = extractKeywords(result.Content)
	result.Language = detectLanguage(result.Content)
	result.Quality = calculateQuality(result.Title, result.Content, task.URL)
	result.Success = true

	return result
}

func (cm *CrawlerMaster) monitorSystem() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			totalTasks := atomic.LoadInt64(&cm.totalTasks)
			totalResults := atomic.LoadInt64(&cm.totalResults)

			activeWorkers := 0
			for _, worker := range cm.workers {
				if worker.running.Load() {
					activeWorkers++
				}
			}

			fmt.Printf("Crawler: %d tasks, %d results, %d active workers\n",
				totalTasks, totalResults, activeWorkers)

		case <-cm.ctx.Done():
			return
		}
	}
}

func (cm *CrawlerMaster) seedCrawler(urls []string) {
	for _, urlStr := range urls {
		task := CrawlTask{
			URL:       urlStr,
			Depth:     0,
			Priority:  1.0,
			Source:    "seed",
			Timestamp: time.Now(),
		}
		cm.AddTask(task)
	}
}

func (cm *CrawlerMaster) getHighQualitySeeds() []string {
	return []string{
		"https://en.wikipedia.org/wiki/Main_Page",
		"https://news.ycombinator.com",
		"https://www.reddit.com/r/programming",
		"https://stackoverflow.com",
		"https://github.com/trending",
		"https://medium.com",
		"https://www.bbc.com/news",
		"https://www.reuters.com",
		"https://arxiv.org",
		"https://scholar.google.com",
	}
}

func (cm *CrawlerMaster) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_tasks":   atomic.LoadInt64(&cm.totalTasks),
		"total_results": atomic.LoadInt64(&cm.totalResults),
		"num_workers":   cm.numWorkers,
		"worker_stats":  make([]WorkerStats, 0, cm.numWorkers),
	}

	cm.stats.Range(func(key, value interface{}) bool {
		if workerStats, ok := value.(*WorkerStats); ok {
			stats["worker_stats"] = append(stats["worker_stats"].([]WorkerStats), *workerStats)
		}
		return true
	})

	return stats
}

func (cm *CrawlerMaster) isValidURL(urlStr string) bool {
	if len(urlStr) > 2000 {
		return false
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	blocked := []string{
		"facebook.com", "twitter.com", "instagram.com", "tiktok.com",
		"youtube.com", "netflix.com", "spotify.com", "amazon.com",
		".exe", ".zip", ".pdf", ".mp4", ".mp3", ".jpg", ".png", ".gif",
	}

	for _, block := range blocked {
		if strings.Contains(strings.ToLower(urlStr), block) {
			return false
		}
	}

	return true
}

func extractTitle(html string) string {
	titleStart := strings.Index(strings.ToLower(html), "<title>")
	if titleStart == -1 {
		return ""
	}
	titleStart += 7
	titleEnd := strings.Index(strings.ToLower(html[titleStart:]), "</title>")
	if titleEnd == -1 {
		return ""
	}
	return strings.TrimSpace(html[titleStart : titleStart+titleEnd])
}

func extractTextContent(html string) string {
	var buf bytes.Buffer
	inTag := false
	inScript := false
	inStyle := false

	html = strings.ToLower(html)

	for i := 0; i < len(html); i++ {
		char := html[i]

		if char == '<' {
			if i+8 < len(html) && html[i:i+8] == "<script>" {
				inScript = true
			} else if i+7 < len(html) && html[i:i+7] == "<style>" {
				inStyle = true
			} else if i+9 < len(html) && html[i:i+9] == "</script>" {
				inScript = false
				i += 8
				continue
			} else if i+8 < len(html) && html[i:i+8] == "</style>" {
				inStyle = false
				i += 7
				continue
			}
			inTag = true
		} else if char == '>' {
			inTag = false
		} else if !inTag && !inScript && !inStyle {
			if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
				if buf.Len() > 0 && buf.String()[buf.Len()-1] != ' ' {
					buf.WriteByte(' ')
				}
			} else {
				buf.WriteByte(char)
			}
		}
	}

	content := strings.TrimSpace(buf.String())
	if len(content) > 50000 {
		content = content[:50000]
	}

	return content
}

func extractLinks(html, baseURL string) []string {
	var links []string
	linkRegex := regexp.MustCompile(`href=['"]([^'"]+)['"]`)
	matches := linkRegex.FindAllStringSubmatch(html, -1)

	base, _ := url.Parse(baseURL)

	for _, match := range matches {
		if len(match) > 1 {
			link := match[1]
			if parsed, err := url.Parse(link); err == nil {
				if !parsed.IsAbs() && base != nil {
					parsed = base.ResolveReference(parsed)
				}
				if parsed.Scheme == "http" || parsed.Scheme == "https" {
					links = append(links, parsed.String())
				}
			}
		}
		if len(links) >= 50 {
			break
		}
	}

	return links
}

func extractKeywords(content string) []string {
	words := strings.Fields(strings.ToLower(content))
	wordCount := make(map[string]int)

	for _, word := range words {
		word = regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(word, "")
		if len(word) > 3 && len(word) < 20 {
			wordCount[word]++
		}
	}

	type wordFreq struct {
		word  string
		count int
	}

	var sorted []wordFreq
	for word, count := range wordCount {
		if count > 1 {
			sorted = append(sorted, wordFreq{word, count})
		}
	}

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var keywords []string
	for i, wf := range sorted {
		if i >= 10 {
			break
		}
		keywords = append(keywords, wf.word)
	}

	return keywords
}

func detectLanguage(content string) string {
	if len(content) < 100 {
		return "unknown"
	}

	englishWords := []string{"the", "and", "for", "are", "but", "not", "you", "all", "can", "had", "her", "was", "one", "our", "out", "day", "get", "has", "him", "his", "how", "its", "may", "new", "now", "old", "see", "two", "way", "who", "boy", "did", "man", "men", "she", "too", "use"}

	words := strings.Fields(strings.ToLower(content))
	englishCount := 0
	totalWords := 0

	for _, word := range words {
		if len(word) > 2 {
			totalWords++
			for _, engWord := range englishWords {
				if word == engWord {
					englishCount++
					break
				}
			}
		}
		if totalWords >= 100 {
			break
		}
	}

	if totalWords > 0 && float64(englishCount)/float64(totalWords) > 0.1 {
		return "en"
	}

	return "unknown"
}

func calculateQuality(title, content, url string) float64 {
	quality := 0.5

	if len(title) > 10 && len(title) < 100 {
		quality += 0.2
	}

	if len(content) > 500 {
		quality += 0.2
	}

	if strings.Contains(url, "wikipedia.org") || strings.Contains(url, "stackoverflow.com") ||
		strings.Contains(url, "github.com") || strings.Contains(url, "arxiv.org") {
		quality += 0.3
	}

	if strings.Contains(strings.ToLower(content), "advertisement") ||
		strings.Contains(strings.ToLower(content), "click here") {
		quality -= 0.3
	}

	if quality > 1.0 {
		quality = 1.0
	}
	if quality < 0.0 {
		quality = 0.0
	}

	return quality
}

func (cm *CrawlerMaster) isContentRelevant(content, title, query string) bool {
	queryTerms := strings.Fields(strings.ToLower(query))
	contentLower := strings.ToLower(content)
	titleLower := strings.ToLower(title)

	matches := 0
	for _, term := range queryTerms {
		if strings.Contains(contentLower, term) || strings.Contains(titleLower, term) {
			matches++
		}
	}

	// Require at least 50% of query terms to be present
	return matches >= len(queryTerms)/2 && matches > 0
}

func (cm *CrawlerMaster) calculatePageRank(pageURL string) float64 {
	// Basic page rank calculation based on domain authority
	if strings.Contains(pageURL, "wikipedia.org") {
		return 0.9
	}
	if strings.Contains(pageURL, "stackoverflow.com") ||
		strings.Contains(pageURL, "github.com") ||
		strings.Contains(pageURL, "arxiv.org") {
		return 0.8
	}
	if strings.Contains(pageURL, "scholar.google.com") ||
		strings.Contains(pageURL, "britannica.com") {
		return 0.7
	}
	if strings.Contains(pageURL, "coursera.org") ||
		strings.Contains(pageURL, "khanacademy.org") ||
		strings.Contains(pageURL, "developer.mozilla.org") {
		return 0.6
	}

	return 0.5 // Default page rank
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
		if str, ok := val.(string); ok {
			if i, err := strconv.Atoi(str); err == nil {
				return i
			}
		}
	}
	return 0
}

func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		if num, ok := val.(float64); ok {
			return num
		}
		if str, ok := val.(string); ok {
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				return f
			}
		}
	}
	return 0.0
}

func hashURL(url string) string {
	hash := 0
	for _, char := range url {
		hash = hash*31 + int(char)
	}
	return fmt.Sprintf("%x", hash)
}

func CreateCrawlerClientCmd(sockPath string) *CrawlerClientCmd {
	return &CrawlerClientCmd{sockPath: sockPath}
}

func (c *CrawlerClientCmd) Connect() (net.Conn, error) {
	return net.Dial("unix", c.sockPath)
}

func (c *CrawlerClientCmd) AddURL(conn net.Conn, url string) error {
	msg := IPCMessage{
		Type: "ADD_TASK",
		Payload: map[string]interface{}{
			"url":      url,
			"depth":    0,
			"priority": 1.0,
			"source":   "manual",
		},
	}
	data, _ := json.Marshal(msg)
	_, err := conn.Write(append(data, '\n'))
	return err
}

func (c *CrawlerClientCmd) AddBulkURLs(conn net.Conn, urls []string) error {
	msg := IPCMessage{
		Type:    "BULK_ADD",
		Payload: urls,
	}
	data, _ := json.Marshal(msg)
	_, err := conn.Write(append(data, '\n'))
	return err
}

func (c *CrawlerClientCmd) GetStats(conn net.Conn) (map[string]interface{}, error) {
	msg := IPCMessage{
		Type:    "GET_STATS",
		Payload: nil,
	}

	data, _ := json.Marshal(msg)
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		var response IPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			return nil, err
		}

		if stats, ok := response.Payload.(map[string]interface{}); ok {
			return stats, nil
		}
	}

	return nil, fmt.Errorf("failed to get stats")
}

func (cm *CrawlerMaster) searchSharedIndex(query string, limit int) []map[string]interface{} {
	if cm.indexer == nil {
		return []map[string]interface{}{}
	}

	// Use the search engine functionality from the indexer's index
	engine := CreateSearchEngine()
	engine.index = cm.indexer.GetIndex()

	results, _, _ := engine.SearchPaginated(query, 1, limit)

	// Convert to map format for JSON response
	var searchResults []map[string]interface{}
	for _, result := range results {
		searchResults = append(searchResults, map[string]interface{}{
			"url":     result.URL,
			"title":   result.Title,
			"snippet": result.Snippet,
			"score":   result.Score,
		})
	}

	return searchResults
}

func (cm *CrawlerMaster) crawlForQuery(query string, conn net.Conn) {
	// For a real search engine, we need to crawl the web organically
	// Start with high-authority seed sites and crawl outward
	
	seedSites := []string{
		"https://en.wikipedia.org/wiki/Main_Page",
		"https://stackoverflow.com",
		"https://github.com/trending",
		"https://news.ycombinator.com",
		"https://www.reddit.com/r/all",
		"https://medium.com",
		"https://dev.to",
		"https://arxiv.org",
		"https://www.nature.com",
		"https://www.bbc.com/news",
		"https://techcrunch.com",
		"https://arstechnica.com",
	}
	
	// Start aggressive crawling from these seeds
	queryTerms := strings.Fields(strings.ToLower(query))
	
	// Add initial seed tasks
	for _, seed := range seedSites {
		task := CrawlTask{
			URL:       seed,
			Depth:     0,
			Priority:  1.0,
			Source:    "query_seed",
			Timestamp: time.Now(),
		}
		cm.AddTask(task)
	}
	
	// Wait for crawling to find relevant content
	timeout := 8 * time.Second
	deadline := time.After(timeout)
	
	// Check for results periodically
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Search our growing index for the query
			results := cm.searchSharedIndex(query, 10)
			
			if len(results) > 0 {
				// Filter results by relevance to the query terms
				var relevantResults []map[string]interface{}
				for _, result := range results {
					if cm.isResultRelevantToQuery(result, queryTerms) {
						relevantResults = append(relevantResults, result)
					}
				}
				
				if len(relevantResults) >= 3 {
					response, _ := json.Marshal(IPCMessage{
						Type: "SEARCH_RESULTS",
						Payload: map[string]interface{}{
							"results": relevantResults,
							"query":   query,
							"total":   len(relevantResults),
						},
					})
					conn.Write(append(response, '\n'))
					return
				}
			}
			
		case <-deadline:
			// Final search - return whatever we found
			results := cm.searchSharedIndex(query, 10)
			response, _ := json.Marshal(IPCMessage{
				Type: "SEARCH_RESULTS",
				Payload: map[string]interface{}{
					"results": results,
					"query":   query,
					"total":   len(results),
				},
			})
			conn.Write(append(response, '\n'))
			return
		}
	}
}

func (cm *CrawlerMaster) crawlSinglePage(pageURL, query string) map[string]interface{} {
	client := &http.Client{
		Timeout: 5 * time.Second, // Increased timeout
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:    10,
			IdleConnTimeout: 30 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SoulSearch/1.0; +http://soulsearch.ai/bot)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	contentStr := string(content)

	// Skip if content is too small or looks like an error page
	if len(contentStr) < 200 {
		return nil
	}

	title := extractTitle(contentStr)
	if title == "" {
		title = pageURL
	}

	// Extract clean text content
	cleanContent := extractTextContent(contentStr)

	// Skip if we don't have meaningful content
	if len(cleanContent) < 100 {
		return nil
	}

	// Check if content is relevant to the query
	if !cm.isContentRelevant(cleanContent, title, query) {
		return nil
	}

	// Create document and add to shared index immediately
	doc := Document{
		URL:      pageURL,
		Title:    title,
		Content:  cleanContent,
		Length:   len(cleanContent),
		PageRank: cm.calculatePageRank(pageURL),
	}

	// Add to shared index
	sharedIndex := GetGlobalSharedIndex()
	if sharedIndex != nil {
		sharedIndex.AddDocument(doc)
	}

	// Generate snippet for this query
	snippet := cm.generateSnippet(cleanContent, query)

	// Calculate relevance score
	score := cm.calculateRelevanceScore(cleanContent, title, query)

	return map[string]interface{}{
		"url":     pageURL,
		"title":   title,
		"snippet": snippet,
		"score":   score,
	}
}

func (cm *CrawlerMaster) generateSnippet(content, query string) string {
	queryTerms := strings.Fields(strings.ToLower(query))
	sentences := strings.Split(content, ".")

	bestSentence := ""
	maxMatches := 0

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < 20 || len(sentence) > 200 {
			continue
		}

		sentenceLower := strings.ToLower(sentence)
		matches := 0

		for _, term := range queryTerms {
			if strings.Contains(sentenceLower, term) {
				matches++
			}
		}

		if matches > maxMatches {
			maxMatches = matches
			bestSentence = sentence
		}
	}

	if bestSentence != "" {
		return bestSentence + "..."
	}

	// Fallback: return first 150 characters
	if len(content) > 150 {
		return content[:150] + "..."
	}
	return content
}

func (cm *CrawlerMaster) calculateRelevanceScore(content, title, query string) float64 {
	queryTerms := strings.Fields(strings.ToLower(query))
	contentLower := strings.ToLower(content)
	titleLower := strings.ToLower(title)

	score := 0.0

	for _, term := range queryTerms {
		// Title matches are worth more
		titleMatches := strings.Count(titleLower, term)
		score += float64(titleMatches) * 3.0

		// Content matches
		contentMatches := strings.Count(contentLower, term)
		score += float64(contentMatches) * 1.0
	}

	// Normalize by content length
	if len(content) > 0 {
		score = score / float64(len(content)) * 1000
	}

	return score
}

func (cm *CrawlerMaster) discoverURLsFromSearchEngines(searchEngines []string, query string) []string {
	var allURLs []string
	urlSet := make(map[string]bool) // To avoid duplicates
	
	// Channel to collect URLs from each search engine
	urlsChan := make(chan []string, len(searchEngines))
	
	// Parse each search engine in parallel
	for _, searchEngine := range searchEngines {
		go func(searchURL string) {
			urls := cm.extractURLsFromSearchPage(searchURL, query)
			urlsChan <- urls
		}(searchEngine)
	}
	
	// Collect URLs from all search engines
	collected := 0
	timeout := time.After(4 * time.Second)
	
	for collected < len(searchEngines) {
		select {
		case urls := <-urlsChan:
			collected++
			for _, u := range urls {
				if !urlSet[u] && cm.isValidDiscoveredURL(u) {
					urlSet[u] = true
					allURLs = append(allURLs, u)
				}
			}
		case <-timeout:
			goto done
		}
	}
	
done:
	
	return allURLs
}

func (cm *CrawlerMaster) extractURLsFromSearchPage(searchURL, query string) []string {
	var urls []string
	
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return urls
	}
	
	// Set headers to look like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	
	resp, err := client.Do(req)
	if err != nil {
		return urls
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return urls
	}
	
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return urls
	}
	
	contentStr := string(content)
	
	// Different patterns for different search engines
	var patterns []string
	
	if strings.Contains(searchURL, "duckduckgo.com") {
		patterns = []string{
			`<a[^>]+href="([^"]+)"[^>]*class="[^"]*result[^"]*"`,
			`<a[^>]+class="[^"]*result[^"]*"[^>]+href="([^"]+)"`,
			`href="(https?://[^"]+)"[^>]*>[^<]*` + regexp.QuoteMeta(query),
		}
	} else if strings.Contains(searchURL, "startpage.com") {
		patterns = []string{
			`<a[^>]+href="([^"]+)"[^>]*class="[^"]*w-gl-result[^"]*"`,
			`href="(https?://[^"]+)"[^>]*class="[^"]*result`,
		}
	} else {
		// Generic patterns
		patterns = []string{
			`href="(https?://[^"]+)"`,
			`<a[^>]+href="([^"]+)"[^>]*>`,
		}
	}
	
	// Extract URLs using patterns
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		
		matches := re.FindAllStringSubmatch(contentStr, -1)
		for _, match := range matches {
			if len(match) > 1 {
				rawURL := match[1]
				
				// Clean up and validate URL
				if cleanURL := cm.cleanDiscoveredURL(rawURL); cleanURL != "" {
					urls = append(urls, cleanURL)
				}
			}
		}
		
		// Limit URLs per search engine
		if len(urls) >= 20 {
			break
		}
	}
	
	return urls
}

func (cm *CrawlerMaster) cleanDiscoveredURL(rawURL string) string {
	// Handle URL unescaping
	if strings.Contains(rawURL, "%") {
		if unescaped, err := url.QueryUnescape(rawURL); err == nil {
			rawURL = unescaped
		}
	}
	
	// Parse URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	
	// Must be HTTP/HTTPS
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return ""
	}
	
	// Remove query parameters and fragments for cleaner URLs
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""
	
	return parsedURL.String()
}

func (cm *CrawlerMaster) isValidDiscoveredURL(urlStr string) bool {
	if len(urlStr) > 2000 || len(urlStr) < 10 {
		return false
	}
	
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	
	// Block search engines themselves
	searchEngineHosts := []string{
		"google.com", "bing.com", "duckduckgo.com", "startpage.com", 
		"yahoo.com", "yandex.com", "baidu.com",
	}
	
	for _, host := range searchEngineHosts {
		if strings.Contains(strings.ToLower(u.Host), host) {
			return false
		}
	}
	
	// Block unwanted file types
	unwantedExtensions := []string{
		".exe", ".zip", ".pdf", ".mp4", ".mp3", ".avi", ".mov",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg",
		".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
	}
	
	for _, ext := range unwantedExtensions {
		if strings.HasSuffix(strings.ToLower(u.Path), ext) {
			return false
		}
	}
	
	// Block some low-quality domains
	blockedDomains := []string{
		"pinterest.com", "instagram.com", "facebook.com", "twitter.com",
		"tiktok.com", "snapchat.com", "linkedin.com",
	}
	
	for _, domain := range blockedDomains {
		if strings.Contains(strings.ToLower(u.Host), domain) {
			return false
		}
	}
	
	return true
}

func (cm *CrawlerMaster) isResultRelevantToQuery(result map[string]interface{}, queryTerms []string) bool {
	title := strings.ToLower(getString(result, "title"))
	snippet := strings.ToLower(getString(result, "snippet"))
	url := strings.ToLower(getString(result, "url"))
	
	matches := 0
	for _, term := range queryTerms {
		if strings.Contains(title, term) || strings.Contains(snippet, term) || strings.Contains(url, term) {
			matches++
		}
	}
	
	// Require at least one query term match
	return matches > 0
}
