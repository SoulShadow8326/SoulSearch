import React, { useState, useEffect } from 'react';
import { Search, Clock, Globe, ArrowRight } from 'lucide-react';
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

function App() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [timeTaken, setTimeTaken] = useState('');
  const [totalResults, setTotalResults] = useState(0);
  const [page, setPage] = useState(1);
  const [hasSearched, setHasSearched] = useState(false);

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
      } else {
        setResults([]);
        setHasSearched(true);
      }
    } catch (error) {
      setResults([]);
      setHasSearched(true);
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

  return (
    <div className="app">
      <div className="search-container">
        <div className="logo-section">
          <h1 className="logo">SoulSearch</h1>
          <p className="tagline">Find what moves your soul</p>
        </div>

        <form onSubmit={handleSearch} className="search-form">
          <div className="search-box">
            <Search className="search-icon" size={20} />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="Search the web..."
              className="search-input"
              autoFocus
            />
            <button type="submit" className="search-button" disabled={loading}>
              {loading ? (
                <div className="loading-spinner" />
              ) : (
                <ArrowRight size={20} />
              )}
            </button>
          </div>
        </form>

        {hasSearched && (
          <div className="results-section">
            {timeTaken && (
              <div className="search-stats">
                <Clock size={14} />
                <span>Found {totalResults} results in {timeTaken}</span>
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
                    <div className="result-footer">
                      <span className="result-score">Score: {result.score.toFixed(2)}</span>
                    </div>
                  </div>
                ))
              ) : (
                <div className="no-results">
                  <Search size={48} className="no-results-icon" />
                  <h3>No results found</h3>
                  <p>Try different keywords or check your spelling</p>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default App;
