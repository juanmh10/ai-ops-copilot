import React from 'react';
import ReactDOM from 'react-dom/client';
import { ChatWidget } from './components/ChatWidget';
import { ChatDrawer } from './components/ChatDrawer';
import { MessageList } from './components/MessageList';

// Auto-mounting global floating widget on document.body for all dashboard views
export function initCopilotWidget() {
  if (typeof document === 'undefined') return;
  if (document.getElementById('ai-ops-copilot-root')) return;

  const container = document.createElement('div');
  container.id = 'ai-ops-copilot-root';
  document.body.appendChild(container);

  const root = ReactDOM.createRoot(container);
  root.render(React.createElement(ChatWidget, { apiEndpoint: '/api/chat' }));
}

if (typeof window !== 'undefined') {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initCopilotWidget);
  } else {
    initCopilotWidget();
  }
  window.addEventListener('popstate', initCopilotWidget);
  setInterval(initCopilotWidget, 1500);
}

// AppPlugin definition for Grafana integration
export class AppPlugin {
  rootPage: any = null;
  init() {
    initCopilotWidget();
  }
  setRootPage(rootPage: any) {
    this.rootPage = rootPage;
    return this;
  }
}

export const plugin = new AppPlugin().setRootPage(ChatWidget);
export { ChatWidget, ChatDrawer, MessageList };
export default ChatWidget;

