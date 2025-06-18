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
    const colors = [
      'rgba(41,119,245,0.18)',
      'rgba(41,119,245,0.28)',
      'rgba(41,119,245,0.38)',
      'rgba(24,41,48,0.18)',
      'rgba(24,41,48,0.28)'
    ];
    const gradients = [
      ctx.createLinearGradient(0, 0, 0, canvas.height),
      ctx.createLinearGradient(0, 0, 0, canvas.height),
      ctx.createLinearGradient(0, 0, 0, canvas.height)
    ];
    gradients[0].addColorStop(0, '#e3f0ff');
    gradients[0].addColorStop(1, 'rgba(41,119,245,0.12)');
    gradients[1].addColorStop(0, '#b3d0f7');
    gradients[1].addColorStop(1, 'rgba(41,119,245,0.22)');
    gradients[2].addColorStop(0, '#7fa7d9');
    gradients[2].addColorStop(1, 'rgba(41,119,245,0.32)');

    const drawSky = () => {
      const sky = ctx.createLinearGradient(0, 0, 0, canvas.height);
      sky.addColorStop(0, '#eaf6ff');
      sky.addColorStop(1, '#b3d0f7');
      ctx.fillStyle = sky;
      ctx.fillRect(0, 0, canvas.width, canvas.height);
    };

    const animate = () => {
      drawSky();
      time += 0.008;
      // Layered mountains
      for (let layer = 0; layer < 3; layer++) {
        ctx.beginPath();
        ctx.moveTo(0, canvas.height);
        for (let x = 0; x <= canvas.width; x += 2) {
          const freq = 0.004 + layer * 0.002;
          const amp = 60 + layer * 40;
          const speed = time * (0.5 + layer * 0.2);
          const y =
            canvas.height - (180 + layer * 60) -
            Math.sin(x * freq + speed) * amp -
            Math.cos(x * (freq * 0.7) + speed * 0.7) * (amp * 0.4);
          ctx.lineTo(x, y);
        }
        ctx.lineTo(canvas.width, canvas.height);
        ctx.closePath();
        ctx.fillStyle = gradients[layer];
        ctx.globalAlpha = 0.8 - layer * 0.2;
        ctx.fill();
        ctx.globalAlpha = 1;
      }
      // Foreground silhouette
      ctx.beginPath();
      ctx.moveTo(0, canvas.height);
      for (let x = 0; x <= canvas.width; x += 2) {
        const y =
          canvas.height - 60 -
          Math.sin(x * 0.012 + time * 1.2) * 18 -
          Math.cos(x * 0.018 + time * 0.8) * 10;
        ctx.lineTo(x, y);
      }
      ctx.lineTo(canvas.width, canvas.height);
      ctx.closePath();
      ctx.fillStyle = 'rgba(24,41,48,0.32)';
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

  return (
    <div className="App">
      <AbstractBackground />
      {searchView && <SearchBackground />}
      <div className="overlay" style={{ zIndex: 2 }}>
        <div className="header">
          <div className="logo">Logo</div>
          <div className="menu-icons">
            <Globe />
            <Settings />
            <MoreVertical />
          </div>
        </div>
        <div className="search-container">
          <form onSubmit={handleSearch} className="search-form">
            <Search className="search-icon" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleKeyPress}
              placeholder="Search..."
              className="search-input"
            />
          </form>
        </div>
        {hasSearched && (
          <div className="results-info">
            {loading ? (
              <div>Loading...</div>
            ) : (
              <div>
                {totalResults} results found in {timeTaken}
              </div>
            )}
          </div>
        )}
        <div className="results-container">
          {searchView && results.length === 0 && !loading && (
            <div className="no-results">
              No results found. Try a different search.
            </div>
          )}
          {results.map((result, index) => (
            <div key={index} className="result-card">
              <div className="result-header">
                <a href={result.url} className="result-title" target="_blank" rel="noopener noreferrer">
                  {result.title}
                </a>
              </div>
              <div className="result-body">
                <div className="result-snippet">{result.snippet}</div>
                <div className="result-meta">
                  <span className="result-score">Score: {result.score}</span>
                  {result.timestamp && (
                    <span className="result-timestamp">{new Date(result.timestamp).toLocaleString()}</span>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
        {totalPages > 1 && (
          <div className="pagination">
            {Array.from({ length: totalPages }, (_, i) => (
              <button
                key={i}
                onClick={() => setPage(i + 1)}
                className={`page-button ${page === i + 1 ? 'active' : ''}`}
              >
                {i + 1}
              </button>
            ))}
          </div>
        )}
        {searchView && (
          <div className="home-button-container">
            <button onClick={goHome} className="home-button">
              Back to Home
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

export default App;
