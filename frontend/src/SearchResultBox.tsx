import React from 'react';
import { motion } from 'framer-motion';
import Box from './Box';

interface SearchResultBoxProps {
  title: string;
  url: string;
  snippet: string;
  onClick?: () => void;
}

const SearchResultBox: React.FC<SearchResultBoxProps> = ({ title, url, snippet, onClick }) => {
  return (
    <Box style={{ minWidth: 260, maxWidth: 340, minHeight: 120, background: '#e3f0ff' }} onClick={onClick}>
      <motion.a href={url} target="_blank" rel="noopener noreferrer" style={{ color: '#2977F5', fontWeight: 600, fontSize: 18, textDecoration: 'none' }}>
        {title}
      </motion.a>
      <div style={{ color: '#434343', fontSize: 14, marginTop: 8 }}>{snippet}</div>
    </Box>
  );
};

export default SearchResultBox;
