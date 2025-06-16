import React, { useState, useEffect, useRef } from 'react';
import { Search, Globe, Settings, MoreVertical } from 'lucide-react';
import './App.css';

interface SearchResult {
  url: string;
  title: string;
  snippet: string;
  score: number;
  timestamp?: string;
}

interface SearchResponse {
  results: SearchResult[];
  total: number;
  page: number;
  total_pages: number;
  time_taken: string;
}

interface Star {
  x: number;
  y: number;
  vx: number;
  vy: number;
  size: number;
  brightness: number;
  color: string;
}

interface Shape {
  x: number;
  y: number;
  size: number;
  rotation: number;
  speed: number;
  type: number;
}

// Constellation Canvas Component
const AbstractBackground: React.FC = () => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const animationIdRef = useRef<number | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const resizeCanvas = () => {
      canvas.width = window.innerWidth;
      canvas.height = window.innerHeight;
    };

    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);

    const people = [
      { x: 150, speed: 0.3, direction: 1 },
      { x: 400, speed: 0.2, direction: -1 },
      { x: 650, speed: 0.4, direction: 1 }
    ];

    let zipLineCart = { x: 50, speed: 1.5 };

    const animate = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      
      ctx.fillStyle = '#2977F5';

      // Draw smooth mountains extending to top
      ctx.beginPath();
      ctx.moveTo(0, canvas.height);
      
      for (let x = 0; x <= canvas.width; x += 40) {
        const height = canvas.height * 0.3 + Math.sin(x * 0.005) * canvas.height * 0.2 + Math.cos(x * 0.003) * canvas.height * 0.15;
        ctx.lineTo(x, height);
      }
      
      ctx.lineTo(canvas.width, 0);
      ctx.lineTo(0, 0);
      ctx.closePath();
      ctx.fill();

      // Draw houses on mountain slopes
      const housePositions = [120, 280, 420, 580, 720];
      housePositions.forEach(x => {
        if (x < canvas.width) {
          const groundY = canvas.height * 0.3 + Math.sin(x * 0.005) * canvas.height * 0.2 + Math.cos(x * 0.003) * canvas.height * 0.15;
          ctx.fillRect(x, groundY, 30, 20);
          ctx.fillRect(x + 5, groundY - 15, 20, 15);
        }
      });

      // Draw trees
      const treePositions = [100, 200, 350, 500, 650];
      treePositions.forEach(x => {
        if (x < canvas.width) {
          const groundY = canvas.height * 0.3 + Math.sin(x * 0.005) * canvas.height * 0.2 + Math.cos(x * 0.003) * canvas.height * 0.15;
          ctx.fillRect(x - 2, groundY - 15, 4, 15);
          ctx.beginPath();
          ctx.arc(x, groundY - 15, 8, 0, Math.PI * 2);
          ctx.fill();
        }
      });

      // Draw zip line
      ctx.strokeStyle = '#2977F5';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(100, canvas.height * 0.2);
      ctx.lineTo(canvas.width - 100, canvas.height * 0.5);
      ctx.stroke();

      // Draw zip line cart (slower)
      const lineProgress = (zipLineCart.x - 100) / (canvas.width - 200);
      const cartY = canvas.height * 0.2 + lineProgress * (canvas.height * 0.3);
      ctx.fillRect(zipLineCart.x, cartY - 3, 6, 6);

      zipLineCart.x += zipLineCart.speed * 0.3;
      if (zipLineCart.x > canvas.width - 100) {
        zipLineCart.x = 100;
      }

      // Draw walking people (slower)
      people.forEach(person => {
        const groundY = canvas.height * 0.3 + Math.sin(person.x * 0.005) * canvas.height * 0.2 + Math.cos(person.x * 0.003) * canvas.height * 0.15;
        ctx.fillRect(person.x - 2, groundY - 8, 4, 8);
        ctx.beginPath();
        ctx.arc(person.x, groundY - 12, 3, 0, Math.PI * 2);
        ctx.fill();

        person.x += person.speed * person.direction * 0.5;
        if (person.x > canvas.width + 10) person.x = -10;
        if (person.x < -10) person.x = canvas.width + 10;
      });

      animationIdRef.current = requestAnimationFrame(animate);
    };

    animate();

    return () => {
      window.removeEventListener('resize', resizeCanvas);
      if (animationIdRef.current) {
        cancelAnimationFrame(animationIdRef.current);
      }
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className="abstract-canvas"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        width: '100%',
        height: '100%',
        pointerEvents: 'none',
        zIndex: 1
      }}
    />
  );
};

function App() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [timeTaken, setTimeTaken] = useState('');
  const [totalResults, setTotalResults] = useState(0);
  const [page, setPage] = useState(1);
  const [hasSearched, setHasSearched] = useState(false);
  const [searchView, setSearchView] = useState(false);

  const searchAPI = async (searchQuery: string, pageNum: number = 1) => {
    if (!searchQuery.trim()) return;
    
    setLoading(true);
    try {
      const response = await fetch('http://localhost:8080/api/search', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          query: searchQuery,
          page: pageNum,
          limit: 10
        })
      });
      
      if (response.ok) {
        const data: SearchResponse = await response.json();
        setResults(data.results || []);
        setTimeTaken(data.time_taken);
        setTotalResults(data.total);
        setHasSearched(true);
        setSearchView(true);
      } else {
        setResults([]);
        setHasSearched(true);
        setSearchView(true);
      }
    } catch (error) {
      setResults([]);
      setHasSearched(true);
      setSearchView(true);
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    searchAPI(query, 1);
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleSearch(e as any);
    }
  };

  const formatURL = (url: string) => {
    try {
      const urlObj = new URL(url);
      return urlObj.hostname + urlObj.pathname;
    } catch {
      return url;
    }
  };

  const goHome = () => {
    setSearchView(false);
    setHasSearched(false);
    setResults([]);
    setQuery('');
  };

  if (searchView) {
    return (
      <div className="search-results-page">
        <AbstractBackground />
        <div className="search-header">
          <div className="search-header-content">
            <div className="logo-small" onClick={goHome}>SoulSearch</div>
            <form onSubmit={handleSearch} className="search-form-header">
              <div className="search-box-header">
                <Search className="search-icon-header" size={16} />
                <input
                  type="text"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  onKeyPress={handleKeyPress}
                  placeholder=""
                  className="search-input-header"
                />
                <button type="submit" className="search-button-header" disabled={loading}>
                  {loading ? (
                    <div className="loading-spinner-small" />
                  ) : (
                    <Search size={16} />
                  )}
                </button>
              </div>
            </form>
            <div className="header-actions">
              <Settings size={20} className="header-icon" />
              <MoreVertical size={20} className="header-icon" />
            </div>
          </div>
        </div>
        
        <div className="search-results-container">
          <div className="search-tabs">
            <div className="tab active">All</div>
            <div className="tab">Images</div>
            <div className="tab">Videos</div>
            <div className="tab">News</div>
            <div className="tab">Maps</div>
          </div>
          
          {timeTaken && (
            <div className="search-stats">
              About {totalResults.toLocaleString()} results ({timeTaken})
            </div>
          )}

          <div className="results-list">
            {results.length > 0 ? (
              results.map((result, index) => (
                <div key={index} className="result-item">
                  <div className="result-header">
                    <Globe size={16} className="globe-icon" />
                    <span className="result-url">{formatURL(result.url)}</span>
                  </div>
                  <h3 className="result-title">
                    <a href={result.url} target="_blank" rel="noopener noreferrer">
                      {result.title}
                    </a>
                  </h3>
                  <p className="result-snippet">{result.snippet}</p>
                </div>
              ))
            ) : (
              <div className="no-results">
                <Search size={48} className="no-results-icon" />
                <h3>No results found</h3>
                <p>Try different keywords or check your spelling</p>
                <button className="crawl-button" onClick={() => {
                  fetch('http://localhost:8080/api/crawl', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ url: 'https://example.com', max_pages: 100 })
                  });
                  alert('Started crawling. Try searching again in a few seconds.');
                }}>
                  Start Crawling Data
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app">
      <AbstractBackground />
      <div className="home-container">
        <div className="logo-section">
          <h1 className="logo">SoulSearch</h1>
        </div>

        <form onSubmit={handleSearch} className="search-form">
          <div className="search-box">
            <Search className="search-icon" size={20} />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder=""
              className="search-input"
              autoFocus
            />
            <button type="submit" className="search-button" disabled={loading}>
              {loading ? (
                <div className="loading-spinner" />
              ) : (
                <Search size={20} />
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default App;
