import React, { createContext, useContext, useState, useCallback } from 'react';

export type GridCell = {
  id: string;
  row: number;
  col: number;
  content?: React.ReactNode;
  type: 'background' | 'title' | 'search' | 'result' | 'other';
  color: string;
};

interface GridContextType {
  grid: GridCell[][];
  setCellContent: (row: number, col: number, content: React.ReactNode, type: GridCell['type']) => void;
  clearCell: (row: number, col: number) => void;
  rows: number;
  cols: number;
}

const GridContext = createContext<GridContextType | undefined>(undefined);

export const useGrid = () => {
  const ctx = useContext(GridContext);
  if (!ctx) throw new Error('useGrid must be used within GridProvider');
  return ctx;
};

export const GridProvider: React.FC<{ rows: number; cols: number; children: React.ReactNode }> = ({ rows, cols, children }) => {
  const [grid, setGrid] = useState<GridCell[][]>(() => {
    const colors = ['#e3f0ff', '#c7e0fa', '#b3d0f7', '#a5c6ef', '#a9cbe6', '#d6e8fa'];
    return Array.from({ length: rows }, (_, row) =>
      Array.from({ length: cols }, (_, col) => ({
        id: `cell-${row}-${col}`,
        row,
        col,
        type: 'background',
        color: colors[(row * cols + col) % colors.length],
      }))
    );
  });

  const setCellContent = useCallback((row: number, col: number, content: React.ReactNode, type: GridCell['type']) => {
    setGrid(g => g.map((r, i) =>
      i === row ? r.map((cell, j) =>
        j === col ? { ...cell, content, type } : cell
      ) : r
    ));
  }, []);

  const clearCell = useCallback((row: number, col: number) => {
    setGrid(g => g.map((r, i) =>
      i === row ? r.map((cell, j) =>
        j === col ? { ...cell, content: undefined, type: 'background' } : cell
      ) : r
    ));
  }, []);

  return (
    <GridContext.Provider value={{ grid, setCellContent, clearCell, rows, cols }}>
      {children}
    </GridContext.Provider>
  );
};
