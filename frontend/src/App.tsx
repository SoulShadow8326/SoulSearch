import React, { useState, useEffect, useRef } from 'react';
import { Search } from 'lucide-react';
import './App.css';
import { BrowserRouter as Router, Routes, Route, useNavigate, useLocation } from 'react-router-dom';
import Box from './Box';
import SearchResultBox from './SearchResultBox';
import { motion, AnimatePresence } from 'framer-motion';
import InteractiveBoxGrid from './InteractiveBoxGrid';
import BoxGrid from './BoxGrid';
import { GridProvider } from './GridContext';
import { useGridLayout } from './useGridLayout';

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
    <div className="home-container" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', gap: 48 }}>
      <Box style={{ marginBottom: 32, minWidth: 320, background: '#e3f0ff', textAlign: 'center' }}>
        <h1 className="logo" style={{ color: '#2977F5', fontSize: 48, fontWeight: 800, letterSpacing: 1 }}>
          <span className="logo-ex">Soul</span><span className="logo-search">Search</span>
        </h1>
      </Box>
      <Box style={{ minWidth: 320, background: '#b3d0f7', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <form onSubmit={handleSearch} className="search-form" style={{ width: '100%' }}>
          <div className="search-box" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <input
              type="text"
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={handleKeyPress}
              placeholder="Search..."
              className="search-input"
              autoFocus
              style={{ flex: 1, fontSize: 20, border: 'none', outline: 'none', background: 'transparent', color: '#2977F5', fontWeight: 500 }}
            />
            <button type="submit" className="search-button" style={{ background: '#2977F5', color: '#fff', border: 'none', borderRadius: 12, padding: '8px 18px', fontWeight: 600, fontSize: 18 }}>
              <Search size={20} color="#fff" />
            </button>
          </div>
        </form>
      </Box>
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
    <div className="search-results-page" style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', alignItems: 'center', paddingTop: 48 }}>
      <Box style={{ marginBottom: 32, minWidth: 320, background: '#e3f0ff', textAlign: 'center' }}>
        <h1 className="logo" style={{ color: '#2977F5', fontSize: 36, fontWeight: 800, letterSpacing: 1 }}>
          <span className="logo-ex">Soul</span><span className="logo-search">Search</span>
        </h1>
      </Box>
      <Box style={{ minWidth: 320, background: '#b3d0f7', display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 32 }}>
        <form onSubmit={handleSearch} className="search-form" style={{ width: '100%' }}>
          <div className="search-box" style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <input
              type="text"
              value={input}
              onChange={e => setInput(e.target.value)}
              placeholder="Search..."
              className="search-input"
              autoFocus
              style={{ flex: 1, fontSize: 20, border: 'none', outline: 'none', background: 'transparent', color: '#2977F5', fontWeight: 500 }}
            />
            <button type="submit" className="search-button" style={{ background: '#2977F5', color: '#fff', border: 'none', borderRadius: 12, padding: '8px 18px', fontWeight: 600, fontSize: 18 }}>
              <Search size={20} color="#fff" />
            </button>
          </div>
        </form>
      </Box>
      <div style={{ width: '100%', maxWidth: 1200, margin: '0 auto', marginBottom: 32 }}>
        <AnimatePresence>
          {results.length > 0 && results.map((result: any, index: number) => (
            <motion.div key={result.url + index} layout initial={{ opacity: 0, scale: 0.8 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.8 }} transition={{ type: 'spring', stiffness: 200, damping: 24 }}>
              <SearchResultBox title={result.title} url={result.url} snippet={result.snippet} />
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
      {totalPages > 1 && (
        <div className="pagination" style={{ marginTop: 24 }}>
          {Array.from({ length: totalPages }, (_, i) => (
            <button
              key={i}
              onClick={() => setPage(i + 1)}
              className={`page-button ${page === i + 1 ? 'active' : ''}`}
              style={{ color: page === i + 1 ? '#fff' : '#2977F5', background: page === i + 1 ? '#2977F5' : '#fff', borderColor: '#2977F5', borderRadius: 12, fontWeight: 600, fontSize: 16, margin: 2, padding: '6px 18px' }}
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
  );
}

type BoxType = {
  key: string;
  color: string;
  content?: React.ReactNode;
  highlight?: boolean;
};

function AppWithRouter() {
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [timeTaken, setTimeTaken] = useState('');
  const [totalResults, setTotalResults] = useState(0);
  const [page, setPage] = useState(1);
  const [mode, setMode] = useState<'home' | 'search'>('home');
  const [query, setQuery] = useState('');
  const cols = Math.max(5, Math.floor(window.innerWidth / 160));
  const rows = Math.max(5, Math.floor(window.innerHeight / 160));
  const boxSize = Math.floor(window.innerWidth / cols) - 12;
  const gap = 12;
  useGridLayout({ mode, query, results });
  return (
    <BoxGrid size={boxSize} gap={gap} />
  );
}

function App() {
  const cols = Math.max(5, Math.floor(window.innerWidth / 160));
  const rows = Math.max(5, Math.floor(window.innerHeight / 160));
  return (
    <GridProvider rows={rows} cols={cols}>
      <Router>
        <AppWithRouter />
      </Router>
    </GridProvider>
  );
}

export default App;
