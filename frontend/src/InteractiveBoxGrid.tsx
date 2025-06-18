import React from 'react';
import { motion, AnimatePresence } from 'framer-motion';

interface InteractiveBoxGridProps {
  boxes: Array<{
    key: string;
    content?: React.ReactNode;
    color: string;
    highlight?: boolean;
    onClick?: () => void;
  }>;
  columns: number;
  boxSize: number;
  gap: number;
  style?: React.CSSProperties;
}

const InteractiveBoxGrid: React.FC<InteractiveBoxGridProps> = ({ boxes, columns, boxSize, gap, style }) => {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${columns}, ${boxSize}px)`,
        gridAutoRows: `${boxSize}px`,
        gap,
        width: '100vw',
        height: '100vh',
        position: 'absolute',
        top: 0,
        left: 0,
        ...style
      }}
    >
      <AnimatePresence>
        {boxes.map((box, i) => (
          <motion.div
            key={box.key}
            layout
            initial={{ opacity: 0, scale: 0.8 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.8 }}
            transition={{ type: 'spring', stiffness: 200, damping: 24 }}
            style={{
              borderRadius: 18,
              background: box.color,
              boxShadow: box.highlight ? '0 0 0 3px #2977F5' : '0 2px 12px 0 rgba(41,119,245,0.07)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              cursor: box.onClick ? 'pointer' : 'default',
              overflow: 'hidden',
              position: 'relative',
              minWidth: 0,
              minHeight: 0
            }}
            onClick={box.onClick}
          >
            {box.content}
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
};

export default InteractiveBoxGrid;
