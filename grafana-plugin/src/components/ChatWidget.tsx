import React, { useState } from 'react';
import { ChatDrawer } from './ChatDrawer';

interface ChatWidgetProps {
  apiEndpoint?: string;
}

export const ChatWidget: React.FC<ChatWidgetProps> = ({ apiEndpoint }) => {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <>
      {/* Floating launcher button in the bottom right corner */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        style={{
          position: 'fixed',
          bottom: '24px',
          right: '24px',
          width: '56px',
          height: '56px',
          borderRadius: '50%',
          background: 'linear-gradient(135deg, #1f78d1 0%, #115da8 100%)',
          color: '#ffffff',
          border: 'none',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.4)',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          fontSize: '24px',
          zIndex: 9998,
          transition: 'transform 0.2s ease, box-shadow 0.2s ease',
        }}
        aria-label="AI-Ops Copilot"
        title="AI-Ops Copilot"
      >
        {isOpen ? (
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="M5 5l14 14M19 5 5 19" /></svg>
        ) : (
          <svg width="25" height="25" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" aria-hidden="true"><path d="M3 4.5h18v12H8l-5 4v-16Z" /></svg>
        )}
      </button>

      {/* Slide-out Drawer */}
      <ChatDrawer
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        apiEndpoint={apiEndpoint}
      />
    </>
  );
};
