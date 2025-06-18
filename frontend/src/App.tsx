import React, { useState, useEffect, useRef } from 'react';
import { Search } from 'lucide-react';
import './App.css';
import { BrowserRouter as Router, Routes, Route, useNavigate, useLocation } from 'react-router-dom';

const getApiBaseUrl = async () => {
  const res = await fetch('/config.json');
  const config = await res.json();
  return config.API_BASE_URL || 'http://localhost:8080';
};

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
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    const size = Math.max(canvas.width, canvas.height) / 18;
    const gap = size * 0.13;
    const cols = Math.ceil(canvas.width / (size + gap)) + 2;
    const rows = Math.ceil(canvas.height / (size + gap)) + 2;
    const colors = [
      '#e3f0ff', '#c7e0fa', '#b3d0f7', '#a5c6ef', '#a9cbe6', '#d6e8fa'
    ];
    for (let row = 0; row < rows; row++) {
      for (let col = 0; col < cols; col++) {
        const angle = ((row + col) % 2 === 0 ? 0.07 : -0.07) + ((row + col) % 3) * 0.04;
        const x = col * (size + gap) + (row % 2) * (size + gap) * 0.5 - size * 0.2;
        const y = row * (size + gap) - size * 0.2;
        const colorIdx = (row + col + (row % 2)) % colors.length;
        ctx.save();
        ctx.translate(x + size / 2, y + size / 2);
        ctx.rotate(angle);
        ctx.beginPath();
        ctx.moveTo(-size / 2 + 8, -size / 2);
        ctx.lineTo(size / 2 - 8, -size / 2);
        ctx.quadraticCurveTo(size / 2, -size / 2, size / 2, -size / 2 + 8);
        ctx.lineTo(size / 2, size / 2 - 8);
        ctx.quadraticCurveTo(size / 2, size / 2, size / 2 - 8, size / 2);
        ctx.lineTo(-size / 2 + 8, size / 2);
        ctx.quadraticCurveTo(-size / 2, size / 2, -size / 2, size / 2 - 8);
        ctx.lineTo(-size / 2, -size / 2 + 8);
        ctx.quadraticCurveTo(-size / 2, -size / 2, -size / 2 + 8, -size / 2);
        ctx.closePath();
        ctx.globalAlpha = 0.93;
        ctx.fillStyle = colors[colorIdx];
        ctx.fill();
        ctx.restore();
      }
    }
    return () => {
      window.removeEventListener('resize', resizeCanvas);
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

function useQueryParam(key: string) {
  const { search } = useLocation();
  return React.useMemo(() => new URLSearchParams(search).get(key) || '', [search, key]);
}

function HomePage({ onSearch }: { onSearch: (q: string) => void }) {
  const [query, setQuery] = useState('');
  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    onSearch(query);
  };
  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSearch(e as any);
  };
  return (
    <div className="home-container">
      <div className="logo-section">
        <h1 className="logo" style={{ color: '#2977F5' }}>
          <span className="logo-ex">Soul</span><span className="logo-search">Search</span>
        </h1>
      </div>
      <form onSubmit={handleSearch} className="search-form">
        <div className="search-box">
          <input
            type="text"
            value={query}
            onChange={e => setQuery(e.target.value)}
            onKeyDown={handleKeyPress}
            placeholder="Search..."
            className="search-input"
            autoFocus
          />
          <button type="submit" className="search-button">
            <Search size={20} color="#2977F5" />
          </button>
        </div>
      </form>
    </div>
  );
}

function SearchPage({
  searchAPI,
  results,
  loading,
  timeTaken,
  totalResults,
  totalPages,
  page,
  setPage,
  goHome,
}: any) {
  const query = useQueryParam('q');
  const [input, setInput] = useState(query);
  useEffect(() => {
    if (query) searchAPI(query, 1);
  }, [query]);
  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (input.trim()) searchAPI(input, 1);
  };
  return (
    <div className="search-results-page">
      <AbstractBackground />
      <div className="search-results-header">
        <form onSubmit={handleSearch} className="search-form search-form-results">
          <div className="search-box">
            <input
              type="text"
              value={input}
              onChange={e => setInput(e.target.value)}
              placeholder="Search..."
              className="search-input"
              autoFocus
            />
            <button type="submit" className="search-button">
              <Search size={20} color="#2977F5" />
            </button>
          </div>
        </form>
      </div>
      <div className="results-container">
        {results.length > 0 && results.map((result: any, index: number) => (
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
        {totalPages > 1 && (
          <div className="pagination">
            {Array.from({ length: totalPages }, (_, i) => (
              <button
                key={i}
                onClick={() => setPage(i + 1)}
                className={`page-button ${page === i + 1 ? 'active' : ''}`}
                style={{ color: page === i + 1 ? '#fff' : '#2977F5', background: page === i + 1 ? '#2977F5' : '#fff', borderColor: '#2977F5' }}
              >
                {i + 1}
              </button>
            ))}
          </div>
        )}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 40, gap: 24 }}>
          <button onClick={goHome} className="home-button" style={{ background: '#2977F5', color: '#fff', borderRadius: 24, fontWeight: 600, fontSize: 18, padding: '12px 32px', boxShadow: '0 4px 16px rgba(41,119,245,0.13)' }}>
            Back to Home
          </button>
          {results.length > 0 && !loading && (
            <div className="results-info">
              {totalResults} results found in {timeTaken}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function AppWithRouter() {
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [timeTaken, setTimeTaken] = useState('');
  const [totalResults, setTotalResults] = useState(0);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [apiBaseUrl, setApiBaseUrl] = useState('http://localhost:8080');
  const navigate = useNavigate();

  useEffect(() => {
    getApiBaseUrl().then(setApiBaseUrl);
  }, []);

  const searchAPI = async (searchQuery: string, pageNum: number = 1) => {
    if (!searchQuery.trim()) return;
    setLoading(true);
    try {
      const response = await fetch(`${apiBaseUrl}/api/search`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: searchQuery, page: pageNum, limit: 10 })
      });
      if (response.ok) {
        const data: SearchResponse = await response.json();
        setResults(data.results || []);
        setTotalResults(data.total || 0);
        setTotalPages(data.total_pages || 1);
        setTimeTaken(data.time_taken || '');
        setPage(pageNum);
      } else {
        setResults([]);
      }
    } catch {
      setResults([]);
    } finally {
      setLoading(false);
    }
  };

  const goHome = () => {
    navigate('/');
    setResults([]);
    setPage(1);
    setTotalResults(0);
    setTotalPages(1);
    setTimeTaken('');
  };

  const handleSearchRoute = (q: string) => {
    if (q.trim()) navigate(`/search?q=${encodeURIComponent(q)}`);
  };

  return (
    <>
      <AbstractBackground />
      <Routes>
        <Route path="/" element={<HomePage onSearch={handleSearchRoute} />} />
        <Route path="/search" element={
          <SearchPage
            searchAPI={searchAPI}
            results={results}
            loading={loading}
            timeTaken={timeTaken}
            totalResults={totalResults}
            totalPages={totalPages}
            page={page}
            setPage={setPage}
            goHome={goHome}
          />
        } />
      </Routes>
    </>
  );
}

function App() {
  return (
    <Router>
      <AppWithRouter />
    </Router>
  );
}

export default App;
