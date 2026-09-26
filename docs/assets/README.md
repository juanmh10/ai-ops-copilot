# Project Assets & Media Catalog

This directory holds visual assets, architecture diagrams, and dashboard/UI screenshots for the **AI-Ops Copilot** project.

---

## 📁 Directory Structure

```text
docs/assets/
├── architecture/     # High-level architecture, network topologies, Draw.io & SVG files
├── dashboards/       # High-resolution screenshots of provisioned Grafana dashboards
├── copilot/          # UI screenshots of the Copilot drawer, chat interactions, and context badges
└── README.md         # Asset index and inventory (this document)
```

---

## 📸 Asset Inventory & Documentation Placements

| Asset File | Format & Resolution | Context / Section | Status |
| :--- | :--- | :--- | :---: |
| [`dashboards/home-dash.png`](dashboards/home-dash.png) | PNG (1368x648) | Global multi-project overview, 4 KPI stats (Requests, 5xx, AI tokens, Invocations), traffic and error distribution, floating Copilot launcher button | ✅ Active |
| [`dashboards/userper-model.png`](dashboards/userper-model.png) | PNG (1372x381) | Vertex AI token input/output ratio, model invocation share donut, and consumption breakdown across Gemini models | ✅ Active |
| [`dashboards/billing-project.png`](dashboards/billing-project.png) | PNG (1015x214) | FinOps invoice breakdown: 5 monitored projects vs total account billing (gross, credits, net BRL) | ✅ Active |
| [`dashboards/telemetry-operations.png`](dashboards/telemetry-operations.png) | PNG (1363x637) | Project-level telemetry dashboard: production vs staging traffic rates, 5xx errors, p95 latencies, and active instances | ✅ Active |
| [`dashboards/telemetry-routines-storage.png`](dashboards/telemetry-routines-storage.png) | PNG (1357x373) | Scheduled job execution health, Firestore read/write operations per second, and database storage footprint | ✅ Active |
| [`architecture/infra-gcp-firebase.svg`](architecture/infra-gcp-firebase.svg) | SVG (Vector) | Complete multi-project resource and IAM map ([`ARCHITECTURE_DIAGRAM.md`](../../ARCHITECTURE_DIAGRAM.md)) | ✅ Active |
| `copilot/chat-drawer-open.png` | PNG / WebP | Floating chat drawer open inside Grafana UI | ⏳ Pending Upload |

---

## 🎨 Asset Guidelines

1. **Theme Consistency:** Screenshots are captured using Grafana's default **Dark Theme** for clarity and unified aesthetic.
2. **Quality & Aspect Ratio:** Aspect ratios preserve native Grafana grid proportions without distortion or aggressive compression.
3. **Privacy & Redaction:** Real-world secrets and personal emails remain redacted; references adhere to generic enterprise domains and authorized operator standards.
