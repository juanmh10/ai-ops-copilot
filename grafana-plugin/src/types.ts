export interface TimeRange {
  from: string;
  to: string;
}

export interface DashboardContext {
  dashboard_uid?: string;
  dashboard_title?: string;
  time_range?: TimeRange;
  active_panel?: string;
  active_panel_title?: string;
  active_filters?: Record<string, string>;
  scope_level?: 'global' | 'project';
  project_id?: string;
  project_ids?: string[];
  environment?: string;
  service_names?: string[];
}

export interface InvokedTool {
  name: string;
  args: Record<string, any>;
  result?: Record<string, any>;
}

export interface ChatMessage {
  id: string;
  role: 'user' | 'model';
  content: string;
  timestamp: string;
  toolCalls?: InvokedTool[];
}

export interface ChatRequest {
  user_id?: string;
  session_id?: string;
  message: string;
  context?: DashboardContext;
}

export interface ChatSession {
  id: string;
  user_id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export interface ChatResponse {
  response: string;
  session_id: string;
  tool_calls?: InvokedTool[];
}
