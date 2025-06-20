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
  const [dimensions, setDimensions] = useState({ width: window.innerWidth, height: window.innerHeight });
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const searchTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    setTimeout(() => setShowContent(true), 500);
    
    const handleResize = () => {
      setDimensions({ width: window.innerWidth, height: window.innerHeight });
    };
    
    window.addEventListener('resize', handleResize);
    
    return () => {
      window.removeEventListener('resize', handleResize);
    };
  }, [mode]);

  const fetchSuggestions = useCallback(async (searchQuery: string) => {
    if (!searchQuery.trim() || searchQuery.length < 2) {
      setSuggestions([]);
      setShowSuggestions(false);
      return;
    }
    
    try {
      const response = await fetch(`http://localhost:8080/api/suggestions?q=${encodeURIComponent(searchQuery)}`);
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
      const response = await fetch(`http://localhost:8080/api/dynamic-search?q=${encodeURIComponent(searchQuery)}`);
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
    setShowSuggestions(false);
    performSearch(query);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setQuery(value);
    if (mode === 'home') {
      debouncedFetchSuggestions(value);
    }
  };

  const selectSuggestion = (suggestion: string) => {
    setQuery(suggestion);
    setShowSuggestions(false);
    performSearch(suggestion);
  };

  const goHome = () => {
    setMode('home');
    setResults([]);
    setQuery('');
    setError('');
    setSuggestions([]);
    setShowSuggestions(false);
  };

  const GRID_COLS = dimensions.width < 768 ? 6 : 8;
  const GRID_ROWS = dimensions.width < 768 ? 7 : 5;
  const gap = Math.max(6, Math.min(16, dimensions.width / 120));
  const boxWidth = (dimensions.width - (GRID_COLS - 1) * gap) / GRID_COLS;
  const boxHeight = (dimensions.height - (GRID_ROWS - 1) * gap) / GRID_ROWS;

  const createGrid = () => {
    const grid = [];
    
    for (let row = 0; row < GRID_ROWS; row++) {
      for (let col = 0; col < GRID_COLS; col++) {
        const isTitle = row === (GRID_ROWS === 7 ? 2 : 1) && col >= (GRID_COLS === 6 ? 1 : 2) && col <= (GRID_COLS === 6 ? 4 : 5);
        const isSearch = row === (GRID_ROWS === 7 ? 4 : 3) && col >= (GRID_COLS === 6 ? 1 : 2) && col <= (GRID_COLS === 6 ? 4 : 5);
        const isBackButton = mode === 'results' && row === 0 && col === 0;
        const isResultsInfo = mode === 'results' && row === 0 && col === (GRID_COLS - 1);
        const isResultArea = mode === 'results' && row >= (GRID_ROWS === 7 ? 5 : 4) && col >= (GRID_COLS === 6 ? 0 : 1) && col <= (GRID_COLS === 6 ? 5 : 6);
        
        // Color based on row position - lighter at top, darker at bottom
        const rowProgress = row / (GRID_ROWS - 1);
        const getRowColor = (progress: number) => {
          const lightBlues = ['#d6e8fa', '#d6e8fa', '#c7e0fa', '#b8d6f5', '#b3d0f7'];
          const darkBlues = ['#a5c6ef', '#8fb3e8', '#7ba3dc', '#6b9bd2', '#5a8bc8'];
          
          if (progress <= 0.5) {
            // Top half - use light blues
            const index = Math.floor(progress * 2 * lightBlues.length);
            return lightBlues[Math.min(index, lightBlues.length - 1)];
          } else {
            // Bottom half - use dark blues
            const index = Math.floor((progress - 0.5) * 2 * darkBlues.length);
            return darkBlues[Math.min(index, darkBlues.length - 1)];
          }
        };
        
        const rowColor = getRowColor(rowProgress);
        
        let content: React.ReactNode = null;
        
        if (isTitle && col === (GRID_COLS === 6 ? 1 : 2) && showContent) {
          content = (
            <div style={{
              width: '100%',
              height: '100%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              cursor: mode === 'results' ? 'pointer' : 'default'
            }} onClick={mode === 'results' ? goHome : undefined}>
              <span style={{
                fontSize: mode === 'results' ? (dimensions.width < 768 ? 40 : 56) : (dimensions.width < 768 ? 48 : 64),
                fontWeight: 800,
                fontFamily: '"Trebuchet MS", "Lucida Grande", "Lucida Sans Unicode", "Lucida Sans", Tahoma, sans-serif',
                letterSpacing: 1,
                textAlign: 'center',
                lineHeight: 1,
                whiteSpace: 'nowrap'
              }}>
                <span style={{
                  color: 'transparent',
                  WebkitTextStroke: '2px #2977F5'
                }}>Soul</span>
                <span style={{
                  color: '#2977F5',
                  WebkitTextStroke: '2px #2977F5'
                }}>Search</span>
              </span>
            </div>
          );
        }
        
        if (isSearch && col === (GRID_COLS === 6 ? 1 : 2) && showContent) {
          content = (
            <div style={{ 
              width: '100%', 
              height: '100%',
              padding: 16, 
              position: 'relative',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
            <form onSubmit={handleSubmit} style={{ width: '100%', maxWidth: '85%' }}>
              {error && (
                <div style={{
                  color: '#e74c3c',
                  fontSize: 11,
                  marginBottom: 6,
                  textAlign: 'center',
                  fontFamily: 'monospace'
                }}>
                  {error}
                </div>
              )}
              <input
                type="text"
                value={query}
                onChange={handleInputChange}
                placeholder="Search..."
                disabled={loading}
                style={{
                  width: '100%',
                  fontSize: mode === 'results' ? (dimensions.width < 768 ? 13 : 15) : (dimensions.width < 768 ? 15 : 17),
                  border: '1px solid #2977F5',
                  outline: 'none',
                  background: 'transparent',
                  color: '#2977F5',
                  fontWeight: 600,
                  padding: mode === 'results' ? (dimensions.width < 768 ? 8 : 10) : (dimensions.width < 768 ? 10 : 12),
                  borderRadius: 6,
                  marginBottom: 8,
                  fontFamily: 'monospace',
                  opacity: loading ? 0.7 : 1,
                  textAlign: 'center'
                }}
                autoFocus={mode === 'home'}
                onFocus={() => {
                  if (mode === 'home' && query && suggestions.length > 0) {
                    setShowSuggestions(true);
                  }
                }}
                onBlur={() => {
                  setTimeout(() => setShowSuggestions(false), 150);
                }}
              />
              <button
                type="submit"
                disabled={loading || !query.trim()}
                style={{
                  background: 'transparent',
                  color: '#2977F5',
                  border: '1px solid #2977F5',
                  borderRadius: 4,
                  padding: mode === 'results' ? (dimensions.width < 768 ? '4px 8px' : '6px 12px') : (dimensions.width < 768 ? '6px 12px' : '8px 16px'),
                  fontWeight: 700,
                  fontSize: mode === 'results' ? (dimensions.width < 768 ? 10 : 12) : (dimensions.width < 768 ? 12 : 14),
                  width: 'auto',
                  fontFamily: 'monospace',
                  textTransform: 'uppercase',
                  letterSpacing: 1,
                  cursor: loading || !query.trim() ? 'not-allowed' : 'pointer',
                  opacity: loading || !query.trim() ? 0.5 : 1,
                  margin: '0 auto',
                  display: 'block'
                }}
              >
                {loading ? 'Searching...' : 'Search'}
              </button>
              {showSuggestions && mode === 'home' && (
                <div style={{
                  position: 'absolute',
                  top: '100%',
                  left: 0,
                  right: 0,
                  background: 'rgba(255,255,255,0.95)',
                  border: '2px solid rgba(41,119,245,0.2)',
                  borderTop: 'none',
                  borderRadius: '0 0 10px 10px',
                  maxHeight: '160px',
                  overflowY: 'auto',
                  zIndex: 1000,
                  boxShadow: '0 6px 20px rgba(41,119,245,0.15)',
                  backdropFilter: 'blur(8px)'
                }}>
                  {suggestions.map((suggestion, index) => (
                      <div
                        key={index}
                        style={{
                          padding: '12px 16px',
                          cursor: 'pointer',
                          fontFamily: 'monospace',
                          fontSize: '16px',
                          color: '#2977F5',
                          borderBottom: index < suggestions.length - 1 ? '1px solid #e0e0e0' : 'none',
                          transition: 'background 0.2s'
                        }}
                        onMouseDown={(e) => e.preventDefault()}
                        onClick={() => selectSuggestion(suggestion)}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.background = '#f0f8ff';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.background = '#fff';
                        }}
                      >
                        {suggestion}
                      </div>
                    ))}
                </div>
              )}
            </form>
            </div>
          );
        }
        
        if (isBackButton && showContent) {
          content = (
            <button
              onClick={goHome}
              style={{
                background: '#2977F5',
                color: '#fff',
                border: 'none',
                borderRadius: 12,
                padding: '12px 16px',
                fontWeight: 700,
                fontSize: dimensions.width < 768 ? 14 : 16,
                fontFamily: 'monospace',
                cursor: 'pointer',
                width: '100%',
                height: '100%'
              }}
            >
              ← Home
            </button>
          );
        }
        
        if (isResultsInfo && showContent && totalResults > 0) {
          content = (
            <div style={{
              width: '100%',
              height: '100%',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              padding: 8,
              textAlign: 'center'
            }}>
              <div style={{
                color: '#2977F5',
                fontSize: dimensions.width < 768 ? 12 : 14,
                fontWeight: 600,
                fontFamily: 'monospace'
              }}>
                {totalResults} results
                {searchTime && ` in ${searchTime}`}
              </div>
            </div>
          );
        }
        
        if (isResultArea && showContent) {
          const resultIndex = (row - (GRID_ROWS === 7 ? 5 : 4)) * (GRID_COLS === 6 ? 6 : 6) + (GRID_COLS === 6 ? col : col - 1);
          if (loading) {
            content = (
              <div style={{
                width: '100%',
                height: '100%',
                padding: 12,
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center'
              }}>
                <div style={{
                  width: '60%',
                  height: '8px',
                  background: 'linear-gradient(90deg, #e0e0e0 25%, #f0f0f0 50%, #e0e0e0 75%)',
                  backgroundSize: '200% 100%',
                  animation: 'shimmer 1.5s infinite',
                  borderRadius: '4px',
                  marginBottom: '8px'
                }}></div>
                <div style={{
                  width: '80%',
                  height: '6px',
                  background: 'linear-gradient(90deg, #e0e0e0 25%, #f0f0f0 50%, #e0e0e0 75%)',
                  backgroundSize: '200% 100%',
                  animation: 'shimmer 1.5s infinite',
                  borderRadius: '3px',
                  animationDelay: '0.2s'
                }}></div>
              </div>
            );
          } else if (results.length > 0) {
            const result = results[resultIndex];
            if (result) {
              content = (
                <div style={{
                  width: '100%',
                  height: '100%',
                  padding: 12,
                  display: 'flex',
                  flexDirection: 'column',
                  justifyContent: 'space-between',
                  cursor: 'pointer'
                }}
                onClick={() => window.open(result.url, '_blank', 'noopener,noreferrer')}
                >
                  <div style={{
                    color: '#2977F5',
                    fontWeight: 700,
                    fontSize: 16,
                    fontFamily: 'monospace',
                    lineHeight: 1.3,
                    display: 'block',
                    marginBottom: 8,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis'
                  }}>
                    {result.title.length > 35 ? result.title.substring(0, 35) + '...' : result.title}
                  </div>
                  <div style={{
                    color: '#666',
                    fontSize: 12,
                    fontFamily: 'monospace',
                    lineHeight: 1.4,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    flex: 1
                  }}>
                    {result.snippet.length > 75 ? result.snippet.substring(0, 75) + '...' : result.snippet}
                  </div>
                  <div style={{
                    color: '#999',
                    fontSize: 10,
                    fontFamily: 'monospace',
                    marginTop: 4
                  }}>
                    Score: {result.score.toFixed(2)}
                  </div>
                </div>
              );
            }
          } else if (mode === 'results' && !loading) {
            if (resultIndex === (GRID_COLS === 6 ? 18 : 12)) {
              content = (
                <div style={{
                  width: '100%',
                  height: '100%',
                  padding: 12,
                  display: 'flex',
                  flexDirection: 'column',
                  justifyContent: 'center',
                  alignItems: 'center',
                  textAlign: 'center'
                }}>
                  <div style={{
                    color: '#2977F5',
                    fontSize: 24,
                    fontWeight: 800,
                    fontFamily: 'monospace',
                    marginBottom: 8
                  }}>
                    No Results
                  </div>
                  <div style={{
                    color: '#666',
                    fontSize: 14,
                    fontFamily: 'monospace'
                  }}>
                    Try different keywords
                  </div>
                </div>
              );
            }
          }
        }
        
        const width = (isTitle || isSearch) && col >= (GRID_COLS === 6 ? 1 : 2) && col <= (GRID_COLS === 6 ? 4 : 5) ? 
          boxWidth * 4 + gap * 3 : boxWidth;
        const height = boxHeight;
        
        if ((isTitle || isSearch) && col > (GRID_COLS === 6 ? 1 : 2)) continue;
        
        grid.push(
          <div
            key={`${row}-${col}`}
            style={{
              position: 'absolute',
              left: col * (boxWidth + gap),
              top: row * (boxHeight + gap),
              width,
              height,
              background: content ? (isSearch ? '#b3d0f7' : (isTitle ? '#d6e8fa' : rowColor)) : rowColor,
              borderRadius: 18,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              boxShadow: content ? '0 2px 12px 0 rgba(41,119,245,0.07)' : '0 2px 12px 0 rgba(41,119,245,0.07)',
              transition: 'all 0.25s cubic-bezier(0.4, 0, 0.2, 1)',
              cursor: content && isResultArea ? 'pointer' : 'default',
              transform: showContent ? 'scale(1)' : 'scale(0.8)',
              opacity: showContent ? 1 : 0,
              animation: showContent ? `fadeInScale 0.5s cubic-bezier(0.4, 0, 0.2, 1) ${(row * 0.08 + col * 0.03)}s both` : 'none',
              willChange: 'transform, box-shadow'
            }}
            onMouseEnter={(e) => {
              if (!content || loading) {
                e.currentTarget.style.transform = 'scale(1.05)';
                e.currentTarget.style.boxShadow = '0 4px 20px rgba(41,119,245,0.2)';
              } else if (isResultArea && content) {
                e.currentTarget.style.transform = 'scale(1.02)';
                e.currentTarget.style.boxShadow = '0 0 0 3px #2977F5, 0 12px 40px rgba(41,119,245,0.25)';
              }
            }}
            onMouseLeave={(e) => {
              if (!content || loading) {
                e.currentTarget.style.transform = 'scale(1)';
                e.currentTarget.style.boxShadow = '0 2px 12px 0 rgba(41,119,245,0.07)';
              } else if (isResultArea && content) {
                e.currentTarget.style.transform = 'scale(1)';
                e.currentTarget.style.boxShadow = '0 0 0 3px #2977F5, 0 8px 32px rgba(41,119,245,0.15)';
              }
            }}
          >
            {content}
          </div>
        );
      }
    }
    
    return grid;
  };

  const gridWidth = GRID_COLS * boxWidth + (GRID_COLS - 1) * gap;
  const gridHeight = GRID_ROWS * boxHeight + (GRID_ROWS - 1) * gap;

  return (
    <div style={{
      width: '100vw',
      height: '100vh',
      overflow: 'hidden',
      position: 'relative',
      background: '#fff'
    }}>
      <div style={{
        position: 'absolute',
        width: gridWidth,
        height: gridHeight,
        top: '50%',
        left: '50%',
        transform: 'translate(-50%, -50%)'
      }}>
        {createGrid()}
      </div>
    </div>
  );
}

export default App;
