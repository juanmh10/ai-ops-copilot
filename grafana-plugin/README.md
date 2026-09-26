# Grafana Chat Plugin: AI-Ops Copilot

A custom Grafana frontend application plugin that embeds an autonomous Site Reliability Engineering copilot (*Autonomous SRE*) directly into Grafana dashboards.

---

## 🚀 Key Features

- **Floating Launcher Widget:** Injects an unobtrusive floating button into the bottom-right corner of all dashboards.
- **Interactive Chat Drawer:** Modern sliding panel interface providing multi-turn conversation threads, markdown rendering, and visual tool execution cards.
- **Persistent Conversations:** "New Chat" initializes a session in Cloud Firestore; "Recent Chats" reopens saved conversations across page refreshes. Sessions are cryptographically verified against the operator's active Grafana session.
- **Dynamic Context Capture:** With every message submitted, the plugin intercepts and attaches active screen state:
  - `dashboard_uid`: UID of the active dashboard.
  - `time_range`: Active temporal window (e.g., `now-3h` to `now`).
  - `active_panel`: Focused panel ID.
  - `active_panel_title`: Title of the focused panel extracted from the DOM.
  - `active_filters`: Active template variable selections (`var-*`).
  - `scope_level`: Scope classification (`global` vs `project`).
  - `project_id` / `project_ids`: Target Google Cloud project derived from the dashboard metadata.
  - `environment` & `service_names`: Associated environment and Cloud Run services.
- **Same-Origin Secure Communication:** Browser calls `/api/chat` and `/api/sessions` on the same origin via the Go backend reverse proxy. The backend authenticates requests against Grafana's `/api/user` before granting access to session history.

---

## 🛠️ Development & Build Scripts

```bash
# Install dependencies
npm install

# Compile TypeScript and validate types
npm run build

# Run automated context capture unit tests
npm test
```
