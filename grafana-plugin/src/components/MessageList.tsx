import React from 'react';
import { ChatMessage } from '../types';
import { translations } from '../i18n/translations';

interface MessageListProps {
  messages: ChatMessage[];
  loading?: boolean;
}

function tableCells(line: string): string[] {
  return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((cell) => cell.trim());
}

function MessageContent({ content }: { content: string }): JSX.Element {
  const lines = content.split('\n');
  const blocks: React.ReactNode[] = [];
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    const divider = lines[index + 1];
    if (line.startsWith('|') && divider && /^\|[\s:|-]+\|$/.test(divider.trim())) {
      const headers = tableCells(line);
      const rows: string[][] = [];
      index++;
      while (index + 1 < lines.length && lines[index + 1].trim().startsWith('|')) {
        rows.push(tableCells(lines[++index]));
      }
      blocks.push(
        <div key={index} style={{ overflowX: 'auto', margin: '7px 0' }}>
          <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 12 }}>
            <thead><tr>{headers.map((cell, cellIndex) => <th key={cellIndex} style={{ padding: '4px 6px', borderBottom: '1px solid #4c5562', textAlign: cellIndex === 0 ? 'left' : 'right' }}>{cell}</th>)}</tr></thead>
            <tbody>{rows.map((row, rowIndex) => <tr key={rowIndex}>{row.map((cell, cellIndex) => <td key={cellIndex} style={{ padding: '4px 6px', borderBottom: '1px solid #343a42', textAlign: cellIndex === 0 ? 'left' : 'right' }}>{cell}</td>)}</tr>)}</tbody>
          </table>
        </div>,
      );
    } else {
      blocks.push(<div key={index} style={{ minHeight: line ? undefined : 7, whiteSpace: 'pre-wrap' }}>{line}</div>);
    }
  }
  return <>{blocks}</>;
}

export const MessageList: React.FC<MessageListProps> = ({ messages, loading }) => {
  const t = translations;
  return (
    <div style={{ flex: 1, overflowY: 'auto', padding: '12px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
      {messages.length === 0 && (
        <div style={{ textAlign: 'center', color: '#8e8e93', marginTop: '40px', fontSize: '13px' }}>
          <p><strong>{t.ready}</strong></p>
          <p>{t.introduction}</p>
        </div>
      )}

      {messages.map((msg) => {
        const isUser = msg.role === 'user';
        return (
          <div
            key={msg.id}
            style={{
              alignSelf: isUser ? 'flex-end' : 'flex-start',
              maxWidth: '85%',
              background: isUser ? '#1f78d1' : '#22252b',
              color: '#ffffff',
              borderRadius: '8px',
              padding: '10px 14px',
              fontSize: '13px',
              lineHeight: '1.4',
              boxShadow: '0 2px 4px rgba(0,0,0,0.2)',
            }}
          >
            {/* Tool Calls badge if any */}
            {msg.toolCalls && msg.toolCalls.length > 0 && (
              <div style={{ marginBottom: '6px', display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
                {msg.toolCalls.map((t, idx) => (
                  <span
                    key={idx}
                    style={{
                      background: '#111217',
                      color: '#5794f2',
                      fontSize: '10px',
                      padding: '2px 6px',
                      borderRadius: '4px',
                      border: '1px solid #2b2f38',
                    }}
                  >
                    {t.name}
                  </span>
                ))}
              </div>
            )}

            <div><MessageContent content={msg.content} /></div>

            <div
              style={{
                fontSize: '10px',
                color: isUser ? '#b8d5ff' : '#8e8e93',
                textAlign: 'right',
                marginTop: '4px',
              }}
            >
              {new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
            </div>
          </div>
        );
      })}

      {loading && (
        <div
          style={{
            alignSelf: 'flex-start',
            background: '#22252b',
            color: '#8e8e93',
            borderRadius: '8px',
            padding: '8px 12px',
            fontSize: '12px',
          }}
        >
          {t.loading}
        </div>
      )}
    </div>
  );
};
