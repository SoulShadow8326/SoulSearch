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
  const [dominoEffect, setDominoEffect] = useState(false);

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

  useEffect(() => {
    console.log(`MODE CHANGED: ${mode}`);
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
        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
          const data = await response.json();
          setSuggestions(data.suggestions || []);
          setShowSuggestions((data.suggestions || []).length > 0);
        } else {
          console.error('Suggestions error: Expected JSON response but got:', contentType);
          setSuggestions([]);
          setShowSuggestions(false);
        }
      } else {
        console.error('Suggestions error: HTTP', response.status, response.statusText);
        setSuggestions([]);
        setShowSuggestions(false);
      }
    } catch (err) {
      console.error('Suggestions error:', err);
      setSuggestions([]);
      setShowSuggestions(false);
    }
  }, []);

  const performSearch = useCallback(async (searchQuery: string) => {
    if (!searchQuery.trim()) return;
    
    setLoading(true);
    setError('');
    setMode('results');
    
    try {
      console.log('Performing search for:', searchQuery);
      const response = await fetch(`http://localhost:8080/api/dynamic-search?q=${encodeURIComponent(searchQuery)}`);
      console.log('Search response status:', response.status);
      
      if (!response.ok) throw new Error(`Search failed with status: ${response.status}`);
      
      const contentType = response.headers.get('content-type');
      if (contentType && contentType.includes('application/json')) {
        const data: SearchResponse = await response.json();
        console.log('Search data received:', data);
        
        // Clean up the results text
        const cleanedResults = (data.results || []).map(result => ({
          ...result,
          title: decodeText(result.title || ''),
          snippet: decodeText(result.snippet || ''),
          url: result.url || ''
        }));
        
        setResults(cleanedResults);
        setTotalResults(data.total || 0);
        setSearchTime(data.time_taken || '');
        setLoading(false);
        setDominoEffect(false); // Stop domino effect when results are ready
      } else {
        throw new Error('Expected JSON response but got: ' + contentType);
      }
    } catch (err) {
      setError('Search failed. Please try again.');
      console.error('Search error:', err);
      setLoading(false);
      setDominoEffect(false); // Stop domino effect on error
      setMode('home');
    }
  }, []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setShowSuggestions(false);
    
    // Trigger domino effect and start search immediately
    setDominoEffect(true);
    performSearch(query);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setQuery(value);
    if (mode === 'home') {
      fetchSuggestions(value);
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
    setLoading(false);
    setDominoEffect(false);
  };

  const GRID_COLS = dimensions.width < 768 ? 6 : 8;
  const GRID_ROWS = dimensions.width < 768 ? 7 : 5;
  const gap = 0;
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
        
        const rowProgress = row / (GRID_ROWS - 1);
        const getRowColor = (progress: number) => {
          const lightBlues = ['#d6e8fa', '#d6e8fa', '#c7e0fa', '#b8d6f5', '#b3d0f7'];
          const darkBlues = ['#a5c6ef', '#8fb3e8', '#7ba3dc', '#6b9bd2', '#5a8bc8'];
          
          if (progress <= 0.5) {
            const index = Math.floor(progress * 2 * lightBlues.length);
            return lightBlues[Math.min(index, lightBlues.length - 1)];
          } else {
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
              cursor: mode === 'results' ? 'pointer' : 'default',
              transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)'
            }} 
            onClick={mode === 'results' ? goHome : undefined}
            onMouseEnter={(e) => {
              if (mode === 'results') {
                e.currentTarget.style.transform = 'scale(1.05) rotate(-1deg)';
              } else {
                e.currentTarget.style.transform = 'scale(1.02)';
              }
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.transform = 'scale(1) rotate(0deg)';
            }}>
              <span style={{
                fontSize: dimensions.width < 768 ? 48 : 64,
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
                  background: 'rgb(214, 232, 250)',
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
                  background: 'rgb(90, 139, 200)',
                  color: 'white',
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
                          e.currentTarget.style.background = 'linear-gradient(90deg, #f0f8ff, #e6f3ff)';
                          e.currentTarget.style.transform = 'translateX(8px) scale(1.02)';
                          e.currentTarget.style.borderLeft = '4px solid #2977F5';
                          e.currentTarget.style.paddingLeft = '12px';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.background = '#fff';
                          e.currentTarget.style.transform = 'translateX(0px) scale(1)';
                          e.currentTarget.style.borderLeft = 'none';
                          e.currentTarget.style.paddingLeft = '16px';
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
        
        if (isResultArea && showContent) {
          content = null;
        }
        
        const width = (isTitle || isSearch) && col >= (GRID_COLS === 6 ? 1 : 2) && col <= (GRID_COLS === 6 ? 4 : 5) ? 
          boxWidth * 4 + gap * 3 : boxWidth;
        const height = boxHeight;
        
        if ((isTitle || isSearch) && col > (GRID_COLS === 6 ? 1 : 2)) continue;
        
        const bottomUpDelay = (GRID_ROWS - 1 - row) * 80 + col * 20;
        
        // Calculate domino effect delay and scale
        const isDominoRow = dominoEffect && (row === (GRID_ROWS === 7 ? 4 : 3) || row === (GRID_ROWS === 7 ? 5 : 4));
        
        grid.push(
          <div
            key={`${row}-${col}`}
            className={`grid-box ${isDominoRow ? 'domino-bounce' : ''}`}
            style={{
              position: 'absolute',
              left: col * (boxWidth + gap),
              top: row * (boxHeight + gap),
              width,
              height,
              background: content ? (isSearch ? '#b3d0f7' : (isTitle ? '#d6e8fa' : rowColor)) : rowColor,
              borderRadius: 0,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              filter: content ? 'none' : 'blur(1px)',
              transition: 'all 0.25s cubic-bezier(0.4, 0, 0.2, 1)',
              cursor: (content && (isTitle || isSearch)) || (content && isResultArea) ? 'pointer' : 'default',
              opacity: showContent ? (content ? 1 : 0.7) : 0,
              animation: showContent ? `fadeInScale 0.5s cubic-bezier(0.4, 0, 0.2, 1) ${(row * 0.08 + col * 0.03)}s both` : undefined
            }}
            onMouseEnter={(e) => {
              if (content && !loading) {
                if (isResultArea) {
                  e.currentTarget.style.transform = 'scale(1.03) rotate(-0.5deg)';
                  e.currentTarget.style.boxShadow = '0 0 0 4px #2977F5, 0 15px 50px rgba(41,119,245,0.3)';
                  e.currentTarget.style.filter = 'brightness(1.05)';
                }
              }
            }}
            onMouseLeave={(e) => {
              if (content && !loading) {
                if (isResultArea) {
                  e.currentTarget.style.transform = 'scale(1) rotate(0deg)';
                  e.currentTarget.style.boxShadow = '0 0 0 3px #2977F5, 0 8px 32px rgba(41,119,245,0.15)';
                  e.currentTarget.style.filter = 'brightness(1)';
                }
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

  // Function to decode Unicode escape sequences and clean up text
  const decodeText = (text: string): string => {
    if (!text) return '';
    
    try {
      let decoded = text;
      
      // Handle JSON-escaped characters
      decoded = decoded.replace(/\\"/g, '"');
      decoded = decoded.replace(/\\'/g, "'");
      decoded = decoded.replace(/\\n/g, ' ');
      decoded = decoded.replace(/\\r/g, ' ');
      decoded = decoded.replace(/\\t/g, ' ');
      
      // Decode Unicode escape sequences
      decoded = decoded.replace(/\\u([0-9a-fA-F]{4})/g, (match, p1) => {
        return String.fromCharCode(parseInt(p1, 16));
      });
      
      // Remove HTML tags more aggressively
      decoded = decoded.replace(/<\/?[^>]+(>|$)/g, '');
      decoded = decoded.replace(/&[a-zA-Z0-9#]+;/g, ' ');
      
      // Remove common HTML artifacts
      decoded = decoded.replace(/\s*-\s*Search results\s*-\s*/gi, ' - ');
      decoded = decoded.replace(/Word stemming is applied/gi, '');
      decoded = decoded.replace(/TOP RESULT/gi, '');
      
      // Clean up extra whitespace
      decoded = decoded.replace(/\s+/g, ' ').trim();
      
      // Remove remaining backslashes that are artifacts
      decoded = decoded.replace(/\\+/g, '');
      
      return decoded;
    } catch (error) {
      console.error('Error decoding text:', error);
      return text.replace(/\\u[0-9a-fA-F]{4}/g, '').replace(/<[^>]*>/g, '').replace(/\s+/g, ' ').trim();
    }
  };

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
      
      {mode === 'results' && !loading && results.length > 0 && (
        <div style={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          height: '50vh',
          background: 'linear-gradient(180deg, #e8f0fe 0%, #f8f9fa 50%, #ffffff 100%)',
          border: '4px solid #2977F5',
          borderBottom: 'none',
          padding: '20px',
          zIndex: 1000,
          animation: 'slideUp 0.5s ease-out',
          boxShadow: '0 -8px 0px rgba(41,119,245,0.3)',
          display: 'flex',
          flexDirection: 'column'
        }}>
          <div style={{
            color: '#2977F5',
            fontSize: '24px',
            fontWeight: 800,
            fontFamily: 'Trebuchet MS, monospace',
            marginBottom: '20px',
            textAlign: 'center',
            textShadow: '2px 2px 0px rgba(41,119,245,0.2)'
          }}>
            SEARCH RESULTS
          </div>
          
          <div style={{
            display: 'flex',
            gap: '20px',
            height: 'calc(100% - 120px)',
            maxWidth: '1400px',
            margin: '0 auto',
            width: '100%',
            padding: '0 20px',
            justifyContent: 'center'
          }}>
            {results[0] && (
              <div style={{
                flex: '0 0 50%',
                background: '#fff',
                border: '4px solid #2977F5',
                padding: '20px',
                cursor: 'pointer',
                transition: 'all 0.25s cubic-bezier(0.4, 0, 0.2, 1)',
                boxShadow: '6px 6px 0px rgba(41,119,245,0.3)',
                transform: 'translate(0, 0)',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center'
              }}
              onClick={() => window.open(results[0].url, '_blank', 'noopener,noreferrer')}
              onMouseEnter={(e) => {
                e.currentTarget.style.transform = 'translate(-5px, -5px) scale(1.02) rotate(-0.5deg)';
                e.currentTarget.style.boxShadow = '12px 12px 0px rgba(41,119,245,0.5)';
                e.currentTarget.style.background = 'linear-gradient(135deg, #f8f9fa, #e6f3ff)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.transform = 'translate(0, 0) scale(1) rotate(0deg)';
                e.currentTarget.style.boxShadow = '6px 6px 0px rgba(41,119,245,0.3)';
                e.currentTarget.style.background = '#fff';
              }}
              >
                <div style={{
                  color: '#2977F5',
                  fontWeight: 800,
                  fontSize: '20px',
                  fontFamily: 'Trebuchet MS, monospace',
                  lineHeight: 1.2,
                  marginBottom: '12px',
                  textShadow: '2px 2px 0px rgba(41,119,245,0.2)',
                  textAlign: 'center'
                }}>
                  {results[0].title}
                </div>
                <div style={{
                  color: '#444',
                  fontSize: '14px',
                  fontFamily: 'monospace',
                  lineHeight: 1.4,
                  marginBottom: '16px',
                  textAlign: 'center',
                  fontWeight: 500
                }}>
                  {results[0].snippet}
                </div>
                <div style={{
                  display: 'flex',
                  justifyContent: 'center',
                  alignItems: 'center',
                  gap: '16px',
                  borderTop: '3px solid #2977F5',
                  paddingTop: '12px'
                }}>
                  <div style={{
                    color: '#2977F5',
                    fontSize: '12px',
                    fontFamily: 'monospace',
                    fontWeight: 700,
                    textTransform: 'uppercase'
                  }}>
                    TOP RESULT
                  </div>
                  <div style={{
                    color: '#fff',
                    fontSize: '12px',
                    fontFamily: 'monospace',
                    fontWeight: 600,
                    background: '#2977F5',
                    padding: '4px 8px',
                    boxShadow: '2px 2px 0px rgba(0,0,0,0.2)'
                  }}>
                    {results[0].score.toFixed(2)}
                  </div>
                </div>
              </div>
            )}
            
            {results.length > 1 && (
              <div style={{
                flex: '0 0 50%',
                display: 'flex',
                flexDirection: 'column',
                gap: '12px',
                overflowY: 'hidden'
              }}>
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr',
                  gap: '12px',
                  height: '100%',
                  alignContent: 'start'
                }}>
                  {results.slice(1, 5).map((result, index) => (
                    <div
                      key={index + 1}
                      style={{
                        background: '#f8f9fa',
                        border: '3px solid #2977F5',
                        padding: '12px',
                        cursor: 'pointer',
                        boxShadow: '3px 3px 0px rgba(41,119,245,0.3)',
                        transform: 'translate(0, 0)',
                        display: 'flex',
                        flexDirection: 'column',
                        minHeight: '120px'
                      }}
                      onClick={() => window.open(result.url, '_blank', 'noopener,noreferrer')}
                    >
                      <div style={{
                        color: '#2977F5',
                        fontWeight: 800,
                        fontSize: '14px',
                        fontFamily: 'Trebuchet MS, monospace',
                        lineHeight: 1.2,
                        marginBottom: '6px',
                        textShadow: '1px 1px 0px rgba(41,119,245,0.2)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical'
                      }}>
                        {result.title}
                      </div>
                      <div style={{
                        color: '#666',
                        fontSize: '11px',
                        fontFamily: 'monospace',
                        lineHeight: 1.3,
                        marginBottom: '8px',
                        fontWeight: 500,
                        flex: 1,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        WebkitLineClamp: 3,
                        WebkitBoxOrient: 'vertical'
                      }}>
                        {result.snippet}
                      </div>
                      <div style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        borderTop: '2px solid #2977F5',
                        paddingTop: '6px'
                      }}>
                        <div style={{
                          color: '#2977F5',
                          fontSize: '9px',
                          fontFamily: 'monospace',
                          fontWeight: 700,
                          textTransform: 'uppercase'
                        }}>
                          #{index + 2}
                        </div>
                        <div style={{
                          color: '#fff',
                          fontSize: '9px',
                          fontFamily: 'monospace',
                          fontWeight: 600,
                          background: '#2977F5',
                          padding: '2px 4px',
                          boxShadow: '1px 1px 0px rgba(0,0,0,0.2)'
                        }}>
                          {result.score.toFixed(2)}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                {results.length > 5 && (
                  <div style={{
                    background: '#2977F5',
                    color: '#fff',
                    padding: '8px',
                    textAlign: 'center',
                    fontFamily: 'Trebuchet MS, monospace',
                    fontWeight: 800,
                    fontSize: '12px',
                    boxShadow: '3px 3px 0px rgba(0,0,0,0.3)',
                    textShadow: '1px 1px 0px rgba(0,0,0,0.3)'
                  }}>
                    +{results.length - 5} MORE RESULTS
                  </div>
                )}
              </div>
            )}
          </div>
          
          <div style={{
            marginTop: 'auto',
            textAlign: 'center',
            paddingTop: '16px'
          }}>
            <button
              onClick={goHome}
              className="back-to-home-button"
            >
              ← BACK TO HOME
            </button>
          </div>
        </div>
      )}
      
      {mode === 'results' && !loading && results.length === 0 && (
        <div style={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          height: '20vh',
          background: 'linear-gradient(180deg, #ffebee 0%, #f8f9fa 50%, #ffffff 100%)',
          border: '4px solid #DC143C',
          borderBottom: 'none',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 1000,
          animation: 'slideUp 0.5s ease-out',
          boxShadow: '0 -8px 0px rgba(220,20,60,0.3)'
        }}>
          <div style={{ 
            textAlign: 'center',
            background: '#fff',
            padding: '24px 32px',
            boxShadow: '6px 6px 0px rgba(0,0,0,0.3)',
            border: 'none'
          }}>
            <div style={{
              color: '#DC143C',
              fontSize: '24px',
              fontWeight: 800,
              fontFamily: 'Trebuchet MS, monospace',
              marginBottom: '8px',
              textShadow: '2px 2px 0px rgba(220,20,60,0.2)'
            }}>
              NO RESULTS FOUND
            </div>
            <div style={{
              color: '#444',
              fontSize: '14px',
              fontFamily: 'monospace',
              fontWeight: 600,
              marginBottom: '16px'
            }}>
              The sloth bear couldn't find anything. Try different keywords!
            </div>
            <button
              onClick={goHome}
              className="back-to-home-button-no-results"
            >
              ← BACK TO HOME
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;
