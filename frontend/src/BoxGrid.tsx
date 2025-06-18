import React from 'react';
import { motion } from 'framer-motion';
import { useGrid, GridCell } from './GridContext';

interface BoxCellProps {
  cell: GridCell;
  size: number;
  gap: number;
}

const BoxCell: React.FC<BoxCellProps> = ({ cell, size, gap }) => {
  return (
    <motion.div
      layout
      style={{
        width: size,
        height: size,
        background: cell.color,
        borderRadius: 18,
        margin: gap / 2,
        boxShadow: cell.type !== 'background' ? '0 0 0 3px #2977F5' : '0 2px 12px 0 rgba(41,119,245,0.07)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        overflow: 'hidden',
        position: 'relative',
        minWidth: 0,
        minHeight: 0
      }}
    >
      {cell.content}
    </motion.div>
  );
};

interface BoxGridProps {
  size: number;
  gap: number;
}

const BoxGrid: React.FC<BoxGridProps> = ({ size, gap }) => {
  const { grid } = useGrid();
  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      width: '100vw',
      height: '100vh',
      position: 'absolute',
      top: 0,
      left: 0,
      zIndex: 1,
      pointerEvents: 'none'
    }}>
      {grid.map((row, i) => (
        <div key={i} style={{ display: 'flex', flexDirection: 'row', width: '100%' }}>
          {row.map(cell => (
            <BoxCell key={cell.id} cell={cell} size={size} gap={gap} />
          ))}
        </div>
      ))}
    </div>
  );
};

export default BoxGrid;
