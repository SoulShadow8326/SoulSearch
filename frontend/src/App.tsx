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

    let time = 0;

    const animate = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      
      time += 0.005;
      
      ctx.fillStyle = '#2977F5';

      // Draw top mountains with slow movement
      ctx.beginPath();
      ctx.moveTo(0, 0);
      for (let x = 0; x <= canvas.width; x += 20) {
        const height = 80 + Math.sin(x * 0.01 + time) * 30 + Math.cos(x * 0.015 + time * 0.7) * 20;
        ctx.lineTo(x, height);
      }
      ctx.lineTo(canvas.width, 0);
      ctx.closePath();
      ctx.fill();

      // Draw bottom mountains with slow movement
      ctx.beginPath();
      ctx.moveTo(0, canvas.height);
      for (let x = 0; x <= canvas.width; x += 20) {
        const height = 120 + Math.sin(x * 0.008 + time * 0.8) * 40 + Math.cos(x * 0.012 + time * 0.6) * 25;
        ctx.lineTo(x, canvas.height - height);
      }
      ctx.lineTo(canvas.width, canvas.height);
      ctx.closePath();
      ctx.fill();

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

const SearchBackground: React.FC = () => {
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

    let time = 0;

    const animate = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      
      time += 0.005;
      
      ctx.fillStyle = '#2977F5';

      ctx.beginPath();
      ctx.moveTo(0, canvas.height);
      for (let x = 0; x <= canvas.width; x += 20) {
        const height = 120 + Math.sin(x * 0.008 + time * 0.8) * 40 + Math.cos(x * 0.012 + time * 0.6) * 25;
        ctx.lineTo(x, canvas.height - height);
      }
      ctx.lineTo(canvas.width, canvas.height);
      ctx.closePath();
      ctx.fill();

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
  const [totalPages, setTotalPages] = useState(1);
  const [hasSearched, setHasSearched] = useState(false);
  const [searchView, setSearchView] = useState(false);

  const searchAPI = async (searchQuery: string, pageNum: number = 1) => {
    if (!searchQuery.trim()) return;
    
    console.log('Starting search for:', searchQuery);
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
      
      console.log('Response status:', response.status);
      
      if (response.ok) {
        const data: SearchResponse = await response.json();
        console.log('Search response data:', data);
        setResults(data.results || []);
        setTotalResults(data.total || 0);
        setTotalPages(data.total_pages || 1);
        setTimeTaken(data.time_taken || '');
        setPage(pageNum);
        setHasSearched(true);
        setSearchView(true);
        console.log('Updated state - results:', data.results?.length, 'searchView:', true);
      } else {
        console.error('Response not ok:', response.status, response.statusText);
        setResults([]);
        setSearchView(true);
      }
    } catch (error) {
      console.error('Search failed:', error);
      setResults([]);
      setSearchView(true);
    } finally {
      setLoading(false);
      console.log('Loading set to false');
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

  const goHome = () => {
    setSearchView(false);
    setQuery('');
    setResults([]);
    setPage(1);
  };

  const goToPage = (pageNum: number) => {
    if (pageNum >= 1 && pageNum <= totalPages) {
      searchAPI(query, pageNum);
    }
  };

  const nextPage = () => {
    if (page < totalPages) {
      goToPage(page + 1);
    }
  };

  const prevPage = () => {
    if (page > 1) {
      goToPage(page - 1);
    }
  };

  const formatURL = (url: string) => {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  };

  if (searchView) {
    console.log('Rendering search view with results:', results, 'length:', results?.length);
    return (
      <div className="search-results-page">
        <SearchBackground />
        <div className="search-header">
          <div className="search-header-content">
            <div className="logo-small" onClick={goHome}>SoulSearch</div>
            <div className="search-center-container">
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
            </div>
            <div className="header-icons">
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
            {results && results.length > 0 ? (
              results.map((result, index) => {
                console.log('Rendering result:', index, result);
                return (
                <div key={index} className="result-item">
                  <div className="result-header">
                    <Globe size={16} className="globe-icon" />
                    <span className="result-url">{formatURL(result.url) || 'No URL'}</span>
                  </div>
                  <h3 className="result-title">
                    <a href={result.url} target="_blank" rel="noopener noreferrer">
                      {result.title || 'No Title'}
                    </a>
                  </h3>
                  <p className="result-snippet">{result.snippet || 'No snippet available'}</p>
                </div>
                );
              })
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

          {totalPages > 1 && (
            <div className="pagination">
              <button className="page-button" onClick={prevPage} disabled={page === 1}>
                &lt; Prev
              </button>
              <span className="current-page">
                Page {page} of {totalPages}
              </span>
              <button className="page-button" onClick={nextPage} disabled={page === totalPages}>
                Next &gt;
              </button>
            </div>
          )}
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
