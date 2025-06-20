package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
