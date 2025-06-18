import { useEffect } from 'react';
import { useGrid } from './GridContext';
import React from 'react';

export function useGridLayout({ mode, query, results }: { mode: 'home' | 'search'; query: string; results: any[] }) {
  const { setCellContent, clearCell, rows, cols } = useGrid();

  useEffect(() => {
    for (let row = 0; row < rows; row++) {
      for (let col = 0; col < cols; col++) {
        clearCell(row, col);
      }
    }
    const titleRow = 1;
    const titleCol = Math.floor(cols / 2);
    setCellContent(titleRow, titleCol, React.createElement('h1', { style: { color: '#2977F5', fontSize: 36, fontWeight: 800, letterSpacing: 1, textAlign: 'center', width: '100%' } }, 'SoulSearch'), 'title');
    const searchRow = 2;
    const searchCol = Math.floor(cols / 2);
    setCellContent(searchRow, searchCol, React.createElement(
      'form',
      {
        onSubmit: (e: React.FormEvent) => { e.preventDefault(); },
        style: { width: '100%' }
      },
      [
        React.createElement('input', {
          key: 'input',
          type: 'text',
          value: query,
          placeholder: 'Search...',
          style: { width: '100%', fontSize: 18, border: 'none', outline: 'none', background: 'transparent', color: '#2977F5', fontWeight: 500, padding: 8 },
          autoFocus: true,
          onChange: () => {}
        }),
        React.createElement('button', {
          key: 'button',
          type: 'submit',
          style: { background: '#2977F5', color: '#fff', border: 'none', borderRadius: 12, padding: '8px 18px', fontWeight: 600, fontSize: 18, marginTop: 8, width: '100%' }
        }, 'Search')
      ]
    ), 'search');
    if (mode === 'search' && results.length > 0) {
      let r = 4;
      let c = 1;
      results.forEach((result: any, i: number) => {
        setTimeout(() => {
          setCellContent(r, c, React.createElement(
            'div',
            { style: { width: '100%', padding: 8 } },
            [
              React.createElement('a', {
                key: 'a',
                href: result.url,
                target: '_blank',
                rel: 'noopener noreferrer',
                style: { color: '#2977F5', fontWeight: 600, fontSize: 18, textDecoration: 'none', wordBreak: 'break-word' }
              }, result.title),
              React.createElement('div', {
                key: 'snippet',
                style: { color: '#434343', fontSize: 14, marginTop: 8 }
              }, result.snippet)
            ]
          ), 'result');
        }, i * 80);
        c++;
        if (c >= cols - 1) {
          c = 1;
          r++;
        }
      });
    }
  }, [mode, query, results, setCellContent, clearCell, rows, cols]);
}
