package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	port   int
	engine *SearchEngine
}

type SearchResponse struct {
	Query     string         `json:"query"`
	Results   []SearchResult `json:"results"`
	Total     int            `json:"total"`
	TimeTaken string         `json:"time_taken"`
}

func NewServer(port int) *Server {
	return &Server{
		port:   port,
		engine: NewSearchEngine(),
	}
}

func (s *Server) Start() {
	http.HandleFunc("/", s.handleHome)
	http.HandleFunc("/search", s.handleSearch)
	http.HandleFunc("/api/search", s.handleAPISearch)
	http.HandleFunc("/static/", s.handleStatic)

	fmt.Printf("SoulSearch running on http://localhost:%d\n", s.port)
	http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SoulSearch</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
        }
        
        .container {
            background: rgba(255, 255, 255, 0.95);
            border-radius: 20px;
            padding: 60px 40px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
            backdrop-filter: blur(10px);
            text-align: center;
            max-width: 600px;
            width: 90%;
        }
        
        .logo {
            font-size: 3.5rem;
            font-weight: 300;
            color: #333;
            margin-bottom: 10px;
            letter-spacing: -2px;
        }
        
        .tagline {
            color: #666;
            margin-bottom: 40px;
            font-size: 1.1rem;
        }
        
        .search-form {
            display: flex;
            gap: 15px;
            margin-bottom: 30px;
        }
        
        .search-input {
            flex: 1;
            padding: 18px 25px;
            border: none;
            border-radius: 50px;
            font-size: 1.1rem;
            background: #f8f9fa;
            outline: none;
            transition: all 0.3s ease;
        }
        
        .search-input:focus {
            background: #fff;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }
        
        .search-btn {
            padding: 18px 35px;
            background: linear-gradient(45deg, #667eea, #764ba2);
            color: white;
            border: none;
            border-radius: 50px;
            font-size: 1.1rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        .search-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 20px rgba(102, 126, 234, 0.3);
        }
        
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
            gap: 20px;
            margin-top: 40px;
        }
        
        .stat {
            text-align: center;
        }
        
        .stat-number {
            font-size: 2rem;
            font-weight: 600;
            color: #667eea;
            display: block;
        }
        
        .stat-label {
            color: #666;
            font-size: 0.9rem;
            margin-top: 5px;
        }
        
        @media (max-width: 768px) {
            .search-form {
                flex-direction: column;
            }
            
            .logo {
                font-size: 2.5rem;
            }
            
            .container {
                padding: 40px 30px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1 class="logo">SoulSearch</h1>
        <p class="tagline">Discover the web with precision and soul</p>
        
        <form class="search-form" action="/search" method="GET">
            <input type="text" name="q" class="search-input" placeholder="Search the indexed web..." required>
            <button type="submit" class="search-btn">Search</button>
        </form>
        
        <div class="stats">
            <div class="stat">
                <span class="stat-number" id="doc-count">Loading...</span>
                <div class="stat-label">Documents</div>
            </div>
            <div class="stat">
                <span class="stat-number" id="term-count">Loading...</span>
                <div class="stat-label">Terms</div>
            </div>
            <div class="stat">
                <span class="stat-number">∞</span>
                <div class="stat-label">Possibilities</div>
            </div>
        </div>
    </div>
    
    <script>
        fetch('/api/search?q=').then(r => r.json()).then(data => {
            document.getElementById('doc-count').textContent = '0';
            document.getElementById('term-count').textContent = '0';
        }).catch(() => {
            document.getElementById('doc-count').textContent = '0';
            document.getElementById('term-count').textContent = '0';
        });
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	start := time.Now()
	results := s.engine.Search(query, 50)
	timeTaken := time.Since(start)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - SoulSearch</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f8f9fa;
            line-height: 1.6;
        }
        
        .header {
            background: white;
            border-bottom: 1px solid #e9ecef;
            padding: 20px 0;
            position: sticky;
            top: 0;
            z-index: 100;
        }
        
        .header-content {
            max-width: 1200px;
            margin: 0 auto;
            padding: 0 20px;
            display: flex;
            align-items: center;
            gap: 30px;
        }
        
        .logo {
            font-size: 1.8rem;
            font-weight: 300;
            color: #667eea;
            text-decoration: none;
            letter-spacing: -1px;
        }
        
        .search-form {
            flex: 1;
            max-width: 600px;
            display: flex;
            gap: 10px;
        }
        
        .search-input {
            flex: 1;
            padding: 12px 20px;
            border: 1px solid #ddd;
            border-radius: 25px;
            font-size: 1rem;
            outline: none;
            transition: all 0.3s ease;
        }
        
        .search-input:focus {
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }
        
        .search-btn {
            padding: 12px 25px;
            background: #667eea;
            color: white;
            border: none;
            border-radius: 25px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        .search-btn:hover {
            background: #5a6fd8;
        }
        
        .main {
            max-width: 1200px;
            margin: 0 auto;
            padding: 30px 20px;
        }
        
        .search-info {
            color: #666;
            margin-bottom: 30px;
            font-size: 0.9rem;
        }
        
        .results {
            display: flex;
            flex-direction: column;
            gap: 25px;
        }
        
        .result {
            background: white;
            border-radius: 12px;
            padding: 25px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.05);
            transition: all 0.3s ease;
        }
        
        .result:hover {
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
            transform: translateY(-2px);
        }
        
        .result-title {
            font-size: 1.3rem;
            font-weight: 500;
            margin-bottom: 8px;
        }
        
        .result-title a {
            color: #1a0dab;
            text-decoration: none;
        }
        
        .result-title a:hover {
            text-decoration: underline;
        }
        
        .result-url {
            color: #006621;
            font-size: 0.9rem;
            margin-bottom: 10px;
            word-break: break-all;
        }
        
        .result-snippet {
            color: #545454;
            line-height: 1.5;
        }
        
        .result-snippet b {
            color: #333;
            font-weight: 600;
        }
        
        .no-results {
            text-align: center;
            padding: 60px 20px;
            color: #666;
        }
        
        .no-results h2 {
            font-size: 1.5rem;
            margin-bottom: 10px;
            color: #333;
        }
        
        @media (max-width: 768px) {
            .header-content {
                flex-direction: column;
                gap: 20px;
            }
            
            .search-form {
                width: 100%%;
            }
            
            .result {
                padding: 20px;
            }
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="header-content">
            <a href="/" class="logo">SoulSearch</a>
            <form class="search-form" action="/search" method="GET">
                <input type="text" name="q" class="search-input" value="%s" required>
                <button type="submit" class="search-btn">Search</button>
            </form>
        </div>
    </div>
    
    <div class="main">
        <div class="search-info">
            About %d results (%s)
        </div>
        
        <div class="results">
`, query, query, len(results), timeTaken.String())

	if len(results) == 0 {
		html += `
            <div class="no-results">
                <h2>No results found</h2>
                <p>Try different keywords or check your spelling</p>
            </div>
        `
	} else {
		for _, result := range results {
			html += fmt.Sprintf(`
            <div class="result">
                <div class="result-title">
                    <a href="%s" target="_blank">%s</a>
                </div>
                <div class="result-url">%s</div>
                <div class="result-snippet">%s</div>
            </div>
            `, result.URL, result.Title, result.URL, result.Snippet)
		}
	}

	html += `
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (s *Server) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	maxResultsStr := r.URL.Query().Get("max")

	maxResults := 10
	if maxResultsStr != "" {
		if parsed, err := strconv.Atoi(maxResultsStr); err == nil && parsed > 0 {
			maxResults = parsed
		}
	}

	start := time.Now()
	results := s.engine.Search(query, maxResults)
	timeTaken := time.Since(start)

	response := SearchResponse{
		Query:     query,
		Results:   results,
		Total:     len(results),
		TimeTaken: timeTaken.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")

	if strings.Contains(path, "..") {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")

	switch {
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	case strings.HasSuffix(path, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	}

	http.ServeFile(w, r, "static/"+path)
}
