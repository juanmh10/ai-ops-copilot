import React, { useEffect, useState } from 'react';
import { ChatMessage, ChatRequest, ChatResponse, ChatSession, DashboardContext } from '../types';
import { MessageList } from './MessageList';
import { captureDashboardContext } from '../context/DashboardContext';
import { translations } from '../i18n/translations';

function getGrafanaUser(): string | undefined {
  const bootData = (window as Window & {
    grafanaBootData?: { user?: { login?: string } };
  }).grafanaBootData;
  return bootData?.user?.login;
}

function endpointUrl(apiEndpoint: string, path: string): string {
  if (apiEndpoint === '/api/chat' && window.location.port === '3000') {
    return `${window.location.protocol}//${window.location.hostname}:8080${path}`;
  }
  const endpoint = new URL(apiEndpoint, window.location.href);
  endpoint.pathname = endpoint.pathname.replace(/\/api\/chat$/, '') + path;
  endpoint.search = '';
  return endpoint.toString();
}

function storageKey(): string {
  return `ai-ops-copilot-active-${getGrafanaUser() || 'user'}`;
}

function saveActiveSession(id: string): void {
  try {
    window.localStorage.setItem(storageKey(), id);
  } catch {
    // The chat still works when storage is disabled.
  }
}

function previousSession(): string {
  try {
    return window.localStorage.getItem(storageKey()) || '';
  } catch {
    return '';
  }
}

function Icon({ name }: { name: 'chat' | 'plus' | 'history' | 'close' }): JSX.Element {
  const paths = {
    chat: <path d="M3 4.5h18v12H8l-5 4v-16Z" />,
    plus: <path d="M12 4v16M4 12h16" />,
    history: <><path d="M3 12a9 9 0 1 0 2.7-6.4" /><path d="M3 4v5h5M12 7v5l3 2" /></>,
    close: <path d="M5 5l14 14M19 5 5 19" />,
  };
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>;
}

interface ChatDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  apiEndpoint?: string;
}

export const ChatDrawer: React.FC<ChatDrawerProps> = ({ isOpen, onClose, apiEndpoint = '/api/chat' }) => {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [sessionID, setSessionID] = useState('');
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingHistory, setLoadingHistory] = useState(false);
  const [showSessions, setShowSessions] = useState(false);
  const [error, setError] = useState('');
  const [activeContext, setActiveContext] = useState<DashboardContext>({});
  const t = translations;

  const headers: HeadersInit = { 'Content-Type': 'application/json' };
  if (window.location.port === '3000') {
    headers['X-Mock-User'] = getGrafanaUser() || 'local-preview';
  }

  async function request(path: string, options: RequestInit = {}): Promise<Response> {
    const response = await fetch(endpointUrl(apiEndpoint, path), {
      credentials: 'include',
      headers,
      ...options,
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(body.error || `HTTP ${response.status}`);
    }
    return response;
  }

  async function refreshSessions(): Promise<ChatSession[]> {
    const response = await request('/api/sessions');
    const saved = (await response.json()) as ChatSession[];
    setSessions(saved || []);
    return saved || [];
  }

  async function loadHistory(id: string): Promise<void> {
    setLoadingHistory(true);
    try {
      const response = await request(`/api/sessions/${encodeURIComponent(id)}/messages`);
      setMessages((await response.json()) as ChatMessage[] || []);
      setSessionID(id);
      saveActiveSession(id);
      setError('');
    } catch {
      setError(t.historyError);
    } finally {
      setLoadingHistory(false);
    }
  }

  useEffect(() => {
    if (!isOpen) return;
    setActiveContext(captureDashboardContext());
    let cancelled = false;
    setLoadingHistory(true);
    request('/api/sessions')
      .then((response) => response.json() as Promise<ChatSession[]>)
      .then(async (saved) => {
        if (cancelled) return;
        setSessions(saved || []);
        const id = saved?.find((session) => session.id === previousSession())?.id || saved?.[0]?.id;
        if (!id) {
          setSessionID('');
          setMessages([]);
          return;
        }
        const response = await request(`/api/sessions/${encodeURIComponent(id)}/messages`);
        if (cancelled) return;
        setSessionID(id);
        setMessages((await response.json()) as ChatMessage[] || []);
        saveActiveSession(id);
      })
      .catch(() => { if (!cancelled) setError(translations.historyError); })
      .finally(() => { if (!cancelled) setLoadingHistory(false); });
    return () => { cancelled = true; };
    // Load once each time the drawer opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, apiEndpoint]);

  async function createSession(): Promise<ChatSession> {
    const response = await request('/api/sessions', { method: 'POST' });
    const session = (await response.json()) as ChatSession;
    setSessions((current) => [session, ...current]);
    setSessionID(session.id);
    saveActiveSession(session.id);
    return session;
  }

  async function handleNewChat(): Promise<void> {
    if (loading || loadingHistory) return;
    setError('');
    try {
      await createSession();
      setMessages([]);
      setInput('');
      setShowSessions(false);
    } catch {
      setError(t.sessionError);
    }
  }

  async function handleSend(): Promise<void> {
    if (!input.trim() || loading || loadingHistory) return;
    const content = input.trim();
    const userMessage: ChatMessage = {
      id: `msg-${Date.now()}`,
      role: 'user',
      content,
      timestamp: new Date().toISOString(),
    };
    setMessages((current) => [...current, userMessage]);
    setInput('');
    setLoading(true);
    setError('');
    const context = captureDashboardContext();
    setActiveContext(context);
    try {
      const id = sessionID || (await createSession()).id;
      const payload: ChatRequest = { session_id: id, message: content, context };
      const response = await request('/api/chat', { method: 'POST', body: JSON.stringify(payload) });
      const data = (await response.json()) as ChatResponse;
      setMessages((current) => [...current, {
        id: `bot-${Date.now()}`,
        role: 'model',
        content: data.response,
        timestamp: new Date().toISOString(),
        toolCalls: data.tool_calls,
      }]);
      try {
        await refreshSessions();
      } catch {
        setError(t.historyError);
      }
    } catch (cause) {
      const detail = cause instanceof Error ? cause.message : t.retry;
      setError(`${t.requestError}: ${detail}`);
    } finally {
      setLoading(false);
    }
  }

  if (!isOpen) return null;

  const iconButton: React.CSSProperties = {
    width: 30, height: 30, display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
    border: '1px solid #3c424e', borderRadius: 5, color: '#c7d0d9', background: '#22252b', cursor: 'pointer',
  };

  return (
    <div style={{ position: 'fixed', top: 0, right: 0, width: 420, maxWidth: '100vw', height: '100vh', background: '#181b1f', borderLeft: '1px solid #2b2f38', boxShadow: '-4px 0 16px rgba(0,0,0,0.5)', zIndex: 9999, display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '12px 16px', borderBottom: '1px solid #2b2f38', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 10, background: '#111217' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 9, minWidth: 0 }}>
          <Icon name="chat" />
          <div>
            <h4 style={{ margin: 0, fontSize: 14, color: '#fff' }}>AI-Ops Copilot</h4>
            <span style={{ fontSize: 11, color: '#73bf69' }}>{t.connected}</span>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <button type="button" onClick={onClose} aria-label={t.close} style={iconButton}><Icon name="close" /></button>
        </div>
      </div>

      <div style={{ padding: '9px 16px', display: 'flex', gap: 8, borderBottom: '1px solid #2b2f38', background: '#161a22' }}>
        <button type="button" onClick={handleNewChat} disabled={loading || loadingHistory} style={{ ...iconButton, width: 'auto', padding: '0 9px', gap: 6 }}><Icon name="plus" />{t.newChat}</button>
        <button type="button" onClick={() => setShowSessions((value) => !value)} aria-expanded={showSessions} disabled={loading} style={{ ...iconButton, width: 'auto', padding: '0 9px', gap: 6 }}><Icon name="history" />{t.recentChats}</button>
      </div>

      {showSessions && (
        <div style={{ maxHeight: 180, overflowY: 'auto', borderBottom: '1px solid #2b2f38', padding: '6px 10px' }}>
          {sessions.length === 0 && <div style={{ padding: 8, color: '#8e8e93', fontSize: 12 }}>{t.noChats}</div>}
          {sessions.map((session) => (
            <button key={session.id} type="button" disabled={loading || loadingHistory} onClick={() => { setShowSessions(false); void loadHistory(session.id); }} style={{ display: 'block', width: '100%', padding: '8px 10px', border: 'none', borderRadius: 4, textAlign: 'left', color: '#fff', background: session.id === sessionID ? '#263746' : 'transparent', cursor: 'pointer' }}>
              <span style={{ display: 'block', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis', fontSize: 12 }}>{session.title || t.untitled}</span>
              <span style={{ display: 'block', color: '#8e8e93', fontSize: 10 }}>{new Date(session.updated_at).toLocaleString()}</span>
            </button>
          ))}
        </div>
      )}

      {activeContext.dashboard_uid && (
        <div style={{ padding: '6px 16px', background: '#161a22', borderBottom: '1px solid #22252b', color: '#8e8e93', fontSize: 11 }}>
          {t.context}: {activeContext.project_id || activeContext.dashboard_uid}
          {activeContext.time_range && <span> · {activeContext.time_range.from} → {activeContext.time_range.to}</span>}
        </div>
      )}
      {error && <div role="alert" style={{ padding: '8px 16px', color: '#ffb357', fontSize: 12, borderBottom: '1px solid #2b2f38' }}>{error}</div>}
      {loadingHistory ? <div style={{ padding: 16, color: '#8e8e93', fontSize: 12 }}>{t.loadingHistory}</div> : <MessageList messages={messages} loading={loading} />}

      <div style={{ padding: '12px 16px', borderTop: '1px solid #2b2f38', display: 'flex', gap: 8, background: '#111217' }}>
        <input type="text" value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void handleSend(); } }} placeholder={t.placeholder} disabled={loading || loadingHistory} style={{ flex: 1, minWidth: 0, padding: '10px 12px', background: '#22252b', border: '1px solid #3c424e', borderRadius: 6, color: '#fff', fontSize: 13, outline: 'none' }} />
        <button type="button" onClick={handleSend} disabled={loading || loadingHistory || !input.trim()} style={{ padding: '0 16px', background: loading || !input.trim() ? '#3c424e' : '#1f78d1', color: '#fff', border: 'none', borderRadius: 6, cursor: loading ? 'not-allowed' : 'pointer', fontWeight: 600, fontSize: 13 }}>{t.send}</button>
      </div>
    </div>
  );
};
