import React from 'react';
import { motion } from 'framer-motion';

interface BoxProps {
  children: React.ReactNode;
  style?: React.CSSProperties;
  className?: string;
  onClick?: () => void;
}

const Box: React.FC<BoxProps> = ({ children, style, className, onClick }) => {
  return (
    <motion.div
      whileHover={{ scale: 1.04 }}
      whileTap={{ scale: 0.98 }}
      transition={{ type: 'spring', stiffness: 300, damping: 20 }}
      className={className}
      style={{
        borderRadius: 18,
        background: '#f6faff',
        boxShadow: '0 2px 12px 0 rgba(41,119,245,0.07)',
        padding: 24,
        cursor: onClick ? 'pointer' : 'default',
        ...style
      }}
      onClick={onClick}
    >
      {children}
    </motion.div>
  );
};

export default Box;
