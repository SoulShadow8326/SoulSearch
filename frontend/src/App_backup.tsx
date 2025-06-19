import React, { useState, useEffect, useCallback, useRef } from 'react';
import './App.css';

interface SearchResult {
  url: string;
  title: string;
  snippet: string;
  score: number;
}

interface SearchResponse {
  results: SearchResult[];
  total: number;
  time_taken: string;
}

function App() {
  const [query, setQuery] = useState('');
  const [showContent, setShowContent] = useState(false);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchTime, setSearchTime] = useState('');
  const [totalResults, setTotalResults] = useState(0);
  const [error, setError] = useState('');
  const [mode, setMode] = useState<'home' | 'results'>('home');
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const searchTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    setTimeout(() => setShowContent(true), 500);
  }, []);

  const fetchSuggestions = useCallback(async (searchQuery: string) => {
    if (!searchQuery.trim() || searchQuery.length < 2) {
      setSuggestions([]);
      setShowSuggestions(false);
      return;
    }
    
    try {
      const response = await fetch(`http://localhost:8080/suggest?q=${encodeURIComponent(searchQuery)}`);
      if (response.ok) {
        const data = await response.json();
        setSuggestions(data.suggestions || []);
        setShowSuggestions((data.suggestions || []).length > 0);
      }
    } catch (err) {
      console.error('Suggestions error:', err);
      setSuggestions([]);
      setShowSuggestions(false);
    }
  }, []);

  const debouncedFetchSuggestions = useCallback((searchQuery: string) => {
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }
    
    searchTimeoutRef.current = setTimeout(() => {
      fetchSuggestions(searchQuery);
    }, 200);
  }, [fetchSuggestions]);

  const performSearch = useCallback(async (searchQuery: string) => {
    if (!searchQuery.trim()) return;
    
    setLoading(true);
    setError('');
    
    try {
      const response = await fetch(`http://localhost:8080/search?q=${encodeURIComponent(searchQuery)}`);
      if (!response.ok) throw new Error('Search failed');
      
      const data: SearchResponse = await response.json();
      setResults(data.results || []);
      setTotalResults(data.total || 0);
      setSearchTime(data.time_taken || '');
      setMode('results');
    } catch (err) {
      setError('Search failed. Please try again.');
      console.error('Search error:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (query.trim() && !loading) {
      setShowSuggestions(false);
      performSearch(query);
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setQuery(value);
    debouncedFetchSuggestions(value);
  };

  const goHome = () => {
    setMode('home');
    setResults([]);
    setError('');
    setShowSuggestions(false);
  };

  if (mode === 'home') {
    return (
      <div className="app">
        <div className="home-container">
          <div className="logo-section">
            <div className="logo">
              <span className="logo-ex">Soul</span>
              <span className="logo-search">Search</span>
            </div>
          </div>

          <form onSubmit={handleSubmit} className="search-form">
            <div className="search-center-container">
              <div className="search-box">
                <div className="search-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <circle cx="11" cy="11" r="8"></circle>
                    <path d="m21 21-4.35-4.35"></path>
                  </svg>
                </div>
                <input
                  type="text"
                  value={query}
                  onChange={handleInputChange}
                  placeholder="Search the web..."
                  disabled={loading}
                  className="search-input"
                />
                <button
                  type="submit"
                  disabled={loading || !query.trim()}
                  className="search-button"
                >
                  {loading ? 'Searching...' : 'Search'}
                </button>
              </div>
            </div>
          </form>

          {showSuggestions && suggestions.length > 0 && (
            <div style={{
              position: 'absolute',
              top: '100%',
              left: '50%',
              transform: 'translateX(-50%)',
              background: 'rgba(255,255,255,0.95)',
              border: '1px solid #2977F5',
              borderRadius: '12px',
              marginTop: '8px',
              zIndex: 1000,
              minWidth: '480px',
              maxWidth: '480px',
              boxShadow: '0 8px 32px rgba(41,119,245,0.15)',
              backdropFilter: 'blur(12px)'
            }}>
              {suggestions.map((suggestion, index) => (
                <div
                  key={index}
                  style={{
                    padding: '12px 16px',
                    cursor: 'pointer',
                    borderBottom: index < suggestions.length - 1 ? '1px solid rgba(41,119,245,0.1)' : 'none',
                    color: '#2977F5',
                    fontFamily: 'monospace',
                    fontSize: '14px',
                    transition: 'background 0.2s'
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.background = '#f0f8ff';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.background = '#fff';
                  }}
                  onClick={() => {
                    setQuery(suggestion);
                    setShowSuggestions(false);
                    performSearch(suggestion);
                  }}
                >
                  {suggestion}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="app">
      <div className="search-results-page">
        <div className="search-results-header">
          <form onSubmit={handleSubmit} className="search-form-results">
            <div className="search-center-container">
              <div className="search-box">
                <div className="search-icon">
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <circle cx="11" cy="11" r="8"></circle>
                    <path d="m21 21-4.35-4.35"></path>
                  </svg>
                </div>
                <input
                  type="text"
                  value={query}
                  onChange={handleInputChange}
                  placeholder="Search the web..."
                  disabled={loading}
                  className="search-input"
                />
                <button
                  type="submit"
                  disabled={loading || !query.trim()}
                  className="search-button"
                >
                  {loading ? 'Searching...' : 'Search'}
                </button>
              </div>
            </div>
          </form>

          <button
            onClick={goHome}
            style={{
              background: 'none',
              border: 'none',
              color: '#2977F5',
              fontSize: '16px',
              cursor: 'pointer',
              marginTop: '16px',
              fontWeight: '600'
            }}
          >
            ← Back to Home
          </button>

          {totalResults > 0 && (
            <div className="results-info">
              Found {totalResults.toLocaleString()} results in {searchTime}
            </div>
          )}
        </div>

        {error && (
          <div style={{
            color: '#e74c3c',
            textAlign: 'center',
            margin: '20px 0',
            fontSize: '18px'
          }}>
            {error}
          </div>
        )}

        {loading && (
          <div style={{
            textAlign: 'center',
            margin: '40px 0',
            color: '#2977F5',
            fontSize: '18px'
          }}>
            Searching...
          </div>
        )}

        {!loading && results.length === 0 && query && (
          <div style={{
            textAlign: 'center',
            margin: '40px 0',
            color: '#666',
            fontSize: '18px'
          }}>
            No results found for "{query}"
          </div>
        )}

        {!loading && results.length > 0 && (
          <div className="results-container">
            {results.map((result, index) => (
              <div key={index} className="result-card">
                <a href={result.url} target="_blank" rel="noopener noreferrer" className="result-title">
                  {result.title}
                </a>
                <div className="result-snippet">{result.snippet}</div>
                <div className="result-meta">
                  <span>{new URL(result.url).hostname}</span>
                  <span>Score: {result.score.toFixed(2)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default App;
