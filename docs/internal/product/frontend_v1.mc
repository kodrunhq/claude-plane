# claude-plane: Frontend Architecture & Design Document

**Version:** 0.1.0-draft
**Author:** José / Claude (Opus)
**Date:** 2026-03-11
**Companion to:** claude-plane Backend Architecture Design Document

---

## 1. Design Philosophy

claude-plane's frontend has two modes, and the UX must transition between them seamlessly:

1. **Command Center** — Dashboard view. You glance at it to see what's running, what finished, which machines are healthy. Designed for a 5-second check from your phone while drinking coffee.
2. **Workbench** — IDE-like view. You're deep in a terminal session, building a job, reviewing a run. Dense, information-rich, keyboard-driven.

The transition isn't a page navigation — it's a zoom. You start at the command center (wide view), click into something (a session, a job, a machine), and the interface smoothly collapses into the workbench. Back button zooms you out.

**Core UX principles:**
- **No training required.** If you've used VS Code, Grafana, or a terminal, you already know how this works.
- **Terminal is sacred.** When you're in a terminal, nothing else competes for attention. Full focus.
- **Status at a glance.** Color-coded badges, not text. Green = good, amber = working, red = problem. You should understand the system state without reading a single word.
- **Keyboard-first, mouse-friendly.** Power users navigate with shortcuts. Everyone else clicks.

---

## 2. Tech Stack

### Core

| Layer | Choice | Rationale |
|-------|--------|-----------|
| **Framework** | React 18+ with TypeScript | Standard, massive ecosystem, you know it |
| **Build tool** | Vite | Fast dev server, clean config, no Webpack pain |
| **Routing** | React Router v6 | File-based-ish routing, nested layouts |
| **State management** | Zustand | Minimal boilerplate, great TypeScript support, no Redux ceremony |
| **Terminal emulator** | xterm.js + xterm-addon-fit + xterm-addon-webgl | Industry standard, WebGL renderer for performance |
| **Styling** | Tailwind CSS + CSS variables for theming | Utility-first, fast iteration, customizable |
| **Icons** | Lucide React | Clean, consistent, tree-shakeable |
| **Charts** | Recharts (health/metrics) | Simple, React-native, good enough for dashboards |
| **DAG visualization** | ReactFlow | Purpose-built for node/edge graphs, interactive, well-maintained |
| **Date/time** | date-fns + date-fns-tz | Lightweight, tree-shakeable, timezone support for cron |
| **WebSocket** | Native WebSocket API + reconnecting-websocket | Thin wrapper for auto-reconnect |
| **HTTP client** | ky (or plain fetch + wrapper) | Lightweight, good defaults, retry support |
| **Notifications** | Sonner | Beautiful toast notifications, minimal config |

### Why NOT these:

| Rejected | Reason |
|----------|--------|
| Next.js / Remix | SSR is overkill — this is a SPA served by the Go binary. No SEO needed. |
| Redux / MobX | Zustand does everything we need with 1/10th the boilerplate. |
| Styled-components / Emotion | CSS-in-JS adds runtime overhead. Tailwind is faster to write and zero runtime. |
| Monaco Editor | Tempting for the "IDE feel" but massive bundle size. We don't need code editing — we need terminal emulation. |
| D3 for DAG | Too low-level. ReactFlow gives us drag-and-drop, zoom, pan, edge routing for free. |
| Socket.io | Overkill. We need raw WebSocket for terminal binary data. Socket.io adds framing overhead. |

### Development Dependencies

| Tool | Purpose |
|------|---------|
| ESLint + Prettier | Code quality |
| Vitest | Unit testing |
| Playwright | E2E testing (optional, for later) |
| Storybook | Component development in isolation (optional, for later) |

---

## 3. Application Shell & Layout System

### 3.1 The Shell

The app has a persistent shell with three zones:

```
┌──────────────────────────────────────────────────────────┐
│  ┌──────┐                                    ┌────────┐  │
│  │ Logo │  claude-plane        [search]  ⚙️  │ user ▾ │  │
│  └──────┘                                    └────────┘  │
├────────┬─────────────────────────────────────────────────┤
│        │                                                 │
│  NAV   │              MAIN CONTENT                       │
│        │                                                 │
│  ○ Cmd │   (changes based on current view)               │
│    Ctr │                                                 │
│        │                                                 │
│  ○ Ses │                                                 │
│    sions                                                 │
│        │                                                 │
│  ○ Jobs│                                                 │
│        │                                                 │
│  ○ Runs│                                                 │
│        │                                                 │
│  ○ Mach│                                                 │
│    ines│                                                 │
│        │                                                 │
│        ├─────────────────────────────────────────────────┤
│        │  STATUS BAR: 3 machines ● | 5 sessions ● |     │
│        │  2 runs active | Last sync: 2s ago              │
└────────┴─────────────────────────────────────────────────┘
```

**Top bar:** Logo, app name, global search (Cmd+K), settings gear, user menu.

**Left nav:** Collapsible sidebar. Icons + labels on desktop, icons-only on laptop, hidden on mobile (hamburger menu). Sections: Command Center, Sessions, Jobs, Runs, Machines.

**Main content:** The entire right area changes based on the active view. When a terminal is open, it takes 100% of this space (or splits into panels).

**Status bar:** Persistent bottom strip. Live system health at a glance. Connection status to the server (green dot = connected, red = disconnected with retry countdown).

### 3.2 Responsive Behavior

| Breakpoint | Sidebar | Terminal | Status bar | Job builder |
|------------|---------|----------|------------|-------------|
| **Desktop** (≥1440px) | Expanded (icons + labels, ~220px) | Full panel, supports split view (2 terminals side by side) | Full detail | Full DAG + step editor side by side |
| **Laptop** (1024–1439px) | Collapsed (icons only, ~56px), expand on hover | Full panel, single terminal | Compact | DAG above, step editor below (stacked) |
| **Tablet** (768–1023px) | Hidden, hamburger toggle | Full screen (no chrome except back button) | Hidden (moved to nav) | Step list only (simplified) |
| **Phone** (<768px) | Hidden, hamburger toggle | Full screen | Hidden | Read-only status view (can't edit jobs, just check progress) |

**The critical insight:** On phone/tablet, you're not building jobs or typing into terminals. You're checking status. The mobile view is a **read-only command center** — sessions running, machines healthy, runs progressing. That's it. Actual work happens on desktop/laptop.

### 3.3 Navigation & Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Cmd+K` | Global search (sessions, jobs, machines — fuzzy match) |
| `Cmd+1` through `Cmd+5` | Switch to nav sections (Command Center, Sessions, Jobs, Runs, Machines) |
| `Cmd+N` | New session (opens machine picker) |
| `Cmd+T` | New terminal tab (if in sessions view) |
| `Cmd+W` | Close current terminal tab (detach, session keeps running) |
| `Cmd+Shift+P` | Command palette (VS Code style — list of all actions) |
| `Cmd+[` / `Cmd+]` | Switch between terminal tabs |
| `Escape` | Back / close modal / exit focus |

**Command palette** is the power-user shortcut. It lists every action: "New session on nuc-01", "Run job: Kodrun V2 Planning", "Kill session sess-abc", "Drain machine nuc-02". Fuzzy-matched, instant.

---

## 4. Views (Pages)

### 4.1 Command Center (`/`)

The landing page. A single-screen overview of the entire system.

```
┌─────────────────────────────────────────────────────────────┐
│  COMMAND CENTER                                              │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  MACHINES                                             │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐              │   │
│  │  │ nuc-01  │  │ nuc-02  │  │ zima-01 │              │   │
│  │  │ ●  3/5  │  │ ●  1/5  │  │ ○  0/3  │              │   │
│  │  │ CPU 34% │  │ CPU 12% │  │ offline │              │   │
│  │  └─────────┘  └─────────┘  └─────────┘              │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌────────────────────────┐  ┌────────────────────────┐     │
│  │  ACTIVE SESSIONS (4)   │  │  ACTIVE RUNS (2)       │     │
│  │                        │  │                        │     │
│  │  sess-a  nuc-01  12m   │  │  Kodrun V2 PRD→TRD    │     │
│  │  sess-b  nuc-01  3h    │  │  ████████░░  Step 2/3  │     │
│  │  sess-c  nuc-01  45s   │  │                        │     │
│  │  sess-d  nuc-02  1h    │  │  Nightly Tests         │     │
│  │                        │  │  ██████████  Complete ✓ │     │
│  └────────────────────────┘  └────────────────────────┘     │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  RECENT ACTIVITY                                      │   │
│  │  09:15  Run "Kodrun V2" step 2 started on nuc-01     │   │
│  │  09:14  Run "Kodrun V2" step 1 completed (exit 0)    │   │
│  │  09:00  Cron triggered "Kodrun V2 Planning"           │   │
│  │  08:45  Session sess-d created (manual) on nuc-02    │   │
│  │  08:30  Machine zima-01 disconnected                  │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Machine cards:** Color-coded border (green/amber/red). Show session count vs. max, CPU gauge. Click to drill into machine detail.

**Active sessions list:** Sorted by duration. Click any to open the terminal. Shows machine, elapsed time, and a tiny activity indicator (is there fresh output, or is it idle?).

**Active runs:** Progress bar per run (steps completed / total steps). Click to open the run detail view.

**Recent activity:** Reverse-chronological feed. Filterable. The "system log" for humans. Auto-updates in real-time via WebSocket.

### 4.2 Sessions View (`/sessions`)

Lists all sessions (active, completed, failed). Two sub-views:

**List view (default):**

```
┌─────────────────────────────────────────────────────────────┐
│  SESSIONS                    [+ New Session]  [Filter ▾]    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ● sess-abc   nuc-01   /repos/kodrun   12m ago       │   │
│  │    Running · Manual · Attached (1 viewer)             │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  ● sess-def   nuc-02   /repos/spark-lens  3h ago     │   │
│  │    Running · Job: "Kodrun V2" Step 2 · Detached       │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  ○ sess-ghi   nuc-01   /repos/bibliostack  Yesterday │   │
│  │    Completed (exit 0) · Manual · 45 min               │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  ✕ sess-jkl   nuc-01   /repos/kodrun   2 days ago    │   │
│  │    Failed (exit 1) · Job: "Nightly Tests" · 12 min    │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Filters: Status [All ▾]  Machine [All ▾]  Type [All ▾]    │
└─────────────────────────────────────────────────────────────┘
```

**Click a session → Terminal view:**

```
┌─────────────────────────────────────────────────────────────┐
│  ← Sessions   sess-abc   nuc-01  /repos/kodrun   ● Live    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                                                       │   │
│  │  $ claude                                             │   │
│  │  Welcome to Claude Code v1.2.3                        │   │
│  │                                                       │   │
│  │  ❯ Analyze the repository structure and identify      │   │
│  │    any architectural issues.                          │   │
│  │                                                       │   │
│  │  I'll start by examining the project structure...     │   │
│  │                                                       │   │
│  │  (full interactive terminal — xterm.js)               │   │
│  │                                                       │   │
│  │                                                       │   │
│  │                                                       │   │
│  │                                                       │   │
│  │                                                       │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ nuc-01 │ 120x40 │ Session: 12m │ Claude Code v1.2.3  │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Terminal chrome:**
- Top bar: back button, session ID, machine, working directory, status badge (● Live / ⏸ Detached / ▶ Replay).
- Bottom bar: machine name, terminal dimensions, session duration, CLI version (if detectable).
- The terminal itself is xterm.js occupying the full remaining space.

**For completed sessions:** The terminal shows in replay mode. A playback control bar appears at the bottom:

```
┌──────────────────────────────────────────────────────────┐
│  ◄◄  ▶  ►►  │  ████████████░░░░░░░  │  12:34 / 45:00   │
│  1x  2x  4x │  (scrubber bar)       │  (elapsed/total)  │
└──────────────────────────────────────────────────────────┘
```

Scrubber bar lets you seek through the session recording. Speed controls for fast-forward. This uses the asciicast v2 data — the frontend parses the timestamped chunks and feeds them into xterm.js at the appropriate rate.

**New Session modal:**

```
┌────────────────────────────────────────┐
│  New Session                       ✕   │
│                                        │
│  Machine:  [nuc-01         ▾]          │
│            3/5 sessions · CPU 34%      │
│                                        │
│  Working directory:                    │
│  [/home/jose/repos/kodrun        ]     │
│                                        │
│  Claude CLI args (optional):           │
│  [--model opus                   ]     │
│                                        │
│            [Cancel]  [Connect →]       │
└────────────────────────────────────────┘
```

Machine dropdown shows health info inline so you can pick wisely. Working directory has autocomplete (agent can list directories via a lightweight RPC — V2 feature, hardcode common paths for V1).

### 4.3 Jobs View (`/jobs`)

Lists all job definitions. This is where you build and manage jobs.

**Job list:**

```
┌─────────────────────────────────────────────────────────────┐
│  JOBS                                      [+ New Job]      │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Kodrun V2 Planning                                   │   │
│  │  3 steps · PRD → TRD → Plan                          │   │
│  │  ⏰ Mon 9:00 (Europe/Madrid) · Last run: 2h ago ✓     │   │
│  │  🔗 On success → "Notify Slack"                       │   │
│  │                              [Run Now]  [Edit]        │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  Nightly Test Suite                                   │   │
│  │  1 step · Run tests                                   │   │
│  │  ⏰ Daily 02:00 (UTC) · Last run: 8h ago ✓            │   │
│  │  🔗 On failure → "Alert Job"                          │   │
│  │                              [Run Now]  [Edit]        │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

Each job card shows: name, step count with a mini pipeline visualization, cron schedule (if any), last run status, cross-job triggers, and quick action buttons.

### 4.4 Job Editor (`/jobs/:id/edit`)

This is the **notebook-like builder**. Two panels: DAG canvas on the left, step editor on the right.

```
┌─────────────────────────────────────────────────────────────┐
│  ← Jobs   Kodrun V2 Planning                [Save] [Run]   │
│                                                              │
│  ┌────────────────────────┬─────────────────────────────┐   │
│  │                        │                             │   │
│  │   DAG CANVAS           │   STEP EDITOR               │   │
│  │   (ReactFlow)          │                             │   │
│  │                        │   Step: Generate PRD         │   │
│  │   ┌───────────┐        │                             │   │
│  │   │ Generate  │        │   Name:                     │   │
│  │   │ PRD       │        │   [Generate PRD          ]  │   │
│  │   └─────┬─────┘        │                             │   │
│  │         │              │   Prompt:                    │   │
│  │         ▼              │   ┌───────────────────────┐  │   │
│  │   ┌───────────┐        │   │ Analyze the Kodrun    │  │   │
│  │   │ Generate  │        │   │ codebase and generate │  │   │
│  │   │ TRD       │        │   │ a comprehensive PRD   │  │   │
│  │   └─────┬─────┘        │   │ covering...           │  │   │
│  │         │              │   └───────────────────────┘  │   │
│  │         ▼              │                             │   │
│  │   ┌───────────┐        │   Machine: [nuc-01      ▾]  │   │
│  │   │ Impl      │        │   Directory: [/repos/kodrun]│   │
│  │   │ Plan      │        │   Timeout: [0 (unlimited) ] │   │
│  │   └───────────┘        │   Expected outputs:         │   │
│  │                        │   [PRD.md                 ] │   │
│  │   [+ Add Step]         │   [+ add output           ] │   │
│  │                        │                             │   │
│  └────────────────────────┴─────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  SCHEDULES & TRIGGERS                                 │   │
│  │                                                       │   │
│  │  ⏰ Cron: [0 9 * * 1    ] TZ: [Europe/Madrid ▾]     │   │
│  │     → "Every Monday at 9:00 AM"  [Enabled ✓]         │   │
│  │                                                       │   │
│  │  🔗 On success → [Select job...  ▾]                   │   │
│  │                                                       │   │
│  │  [+ Add schedule]  [+ Add trigger]                    │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**DAG canvas (left panel):**
- Built with ReactFlow.
- Each step is a node. Dependencies are edges.
- Drag nodes to rearrange. Drag from one node's handle to another to create a dependency.
- Click a node to load its details in the step editor.
- Right-click node for context menu: delete, duplicate, disconnect.
- `[+ Add Step]` button creates a new unconnected node.
- Color-coded node borders match the step's current state when viewing a run (pending=gray, running=blue, completed=green, failed=red).

**Step editor (right panel):**
- Shows when a node is selected.
- The prompt field is a multi-line textarea with enough room to write real instructions (not a single-line input).
- Machine picker with health info.
- Working directory with history/suggestions.
- Timeout in seconds (0 = unlimited).
- Expected outputs: list of file paths the step should produce (used for completion detection and dependency validation).

**Schedules & triggers (bottom panel):**
- Cron expression input with a human-readable translation underneath ("Every Monday at 9:00 AM").
- Timezone dropdown (populated from IANA timezone list, default to user's local timezone).
- Cross-job trigger builder: select condition (on success / on failure / on completion) and target job.

**Laptop responsive:** DAG canvas stacks above step editor (vertical split instead of horizontal).

### 4.5 Runs View (`/runs`)

Lists all job runs, sortable and filterable.

```
┌─────────────────────────────────────────────────────────────┐
│  RUNS                                         [Filter ▾]    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  ● run-001   Kodrun V2 Planning                      │   │
│  │    Triggered: Cron · Started: 09:00 · Running         │   │
│  │    ████████░░░░  Step 2/3 (Generate TRD)              │   │
│  │                                          [View →]     │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  ✓ run-002   Nightly Tests                            │   │
│  │    Triggered: Cron · 02:00–02:12 · Completed          │   │
│  │    ██████████  1/1 steps                              │   │
│  │                                          [View →]     │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  ✕ run-003   Nightly Tests                            │   │
│  │    Triggered: Cron · Yesterday 02:00 · Failed         │   │
│  │    ██████████  1/1 steps (exit code 1)                │   │
│  │    🔗 Triggered: "Alert Job" run-004                   │   │
│  │                                          [View →]     │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  Filters: Job [All ▾]  Status [All ▾]  Trigger [All ▾]     │
└─────────────────────────────────────────────────────────────┘
```

### 4.6 Run Detail View (`/runs/:id`)

This is the **observability view** — where you see exactly what happened (or is happening) in a run.

```
┌─────────────────────────────────────────────────────────────┐
│  ← Runs   run-001   Kodrun V2 Planning   ● Running          │
│  Triggered by: Cron (0 9 * * 1) · Started: 09:00            │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  DAG STATUS                                           │   │
│  │                                                       │   │
│  │   ┌───────────┐     ┌───────────┐     ┌───────────┐  │   │
│  │   │ ✓ PRD     │────►│ ● TRD     │────►│ ○ Plan    │  │   │
│  │   │ 12 min    │     │ Running   │     │ Pending   │  │   │
│  │   │ nuc-01    │     │ nuc-01    │     │ nuc-02    │  │   │
│  │   └───────────┘     └───────────┘     └───────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  STEP DETAIL                                          │   │
│  │                                                       │   │
│  │  [Generate PRD ✓] [Generate TRD ●] [Impl Plan ○]     │   │
│  │                                                       │   │
│  │  ┌────────────────────────────────────────────────┐   │   │
│  │  │                                                │   │   │
│  │  │  (terminal — live for running step,            │   │   │
│  │  │   replay for completed step)                   │   │   │
│  │  │                                                │   │   │
│  │  │  Full xterm.js terminal showing                │   │   │
│  │  │  exactly what Claude is doing                  │   │   │
│  │  │                                                │   │   │
│  │  └────────────────────────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Top section:** DAG rendered as a read-only ReactFlow graph, but with live status colors and timing info on each node. Click a node to scroll the bottom section to that step.

**Bottom section:** Tabbed by step. Each tab shows:
- For a **running** step: the live terminal (xterm.js, you can even type into it to intervene).
- For a **completed** step: the session replay with playback controls.
- For a **pending** step: the prompt and config that will be used.
- For a **failed** step: the session replay + exit code + error context.

This is the killer UX: you see the DAG flowing, click into any step, and get the full terminal experience. Not logs. Not summaries. The actual terminal.

### 4.7 Machines View (`/machines`)

```
┌─────────────────────────────────────────────────────────────┐
│  MACHINES                                                    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  nuc-01                               ● Connected     │   │
│  │                                                       │   │
│  │  Sessions: 3/5       CPU ████████░░ 67%              │   │
│  │  Uptime: 14 days     RAM █████░░░░░ 48%              │   │
│  │  Last seen: 2s ago   Disk ██░░░░░░░ 22%              │   │
│  │                                                       │   │
│  │  Active sessions:                                     │   │
│  │    sess-abc (manual, 12m)                            │   │
│  │    sess-def (job: Kodrun V2, step 2, 8m)             │   │
│  │    sess-ghi (manual, 3h)                             │   │
│  │                                                       │   │
│  │  [New Session]  [Drain]  [View History]               │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  nuc-02                               ● Connected     │   │
│  │  Sessions: 1/5       CPU ██░░░░░░░░ 12%              │   │
│  │  ...                                                  │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │  zima-01                              ○ Disconnected  │   │
│  │  Last seen: 2 hours ago                               │   │
│  │  [Remove from allowlist]                              │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

Resource bars are inline. Sessions list links directly to the terminal. Drain button prevents new sessions while existing ones finish (for maintenance).

---

## 5. Terminal Integration (xterm.js)

This is the core of the product. It must feel native.

### 5.1 Setup

```typescript
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebglAddon } from 'xterm-addon-webgl';
import { Unicode11Addon } from 'xterm-addon-unicode11';

function createTerminal(container: HTMLElement, sessionId: string): Terminal {
  const term = new Terminal({
    cursorBlink: true,
    cursorStyle: 'block',
    fontSize: 14,
    fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", monospace',
    lineHeight: 1.2,
    theme: {
      background: '#0d1117',
      foreground: '#c9d1d9',
      cursor: '#58a6ff',
      selectionBackground: '#264f78',
      // Full 16-color palette tuned for readability
      black: '#0d1117',
      red: '#ff7b72',
      green: '#7ee787',
      yellow: '#d29922',
      blue: '#58a6ff',
      magenta: '#bc8cff',
      cyan: '#39c5cf',
      white: '#b1bac4',
      brightBlack: '#6e7681',
      brightRed: '#ffa198',
      brightGreen: '#56d364',
      brightYellow: '#e3b341',
      brightBlue: '#79c0ff',
      brightMagenta: '#d2a8ff',
      brightCyan: '#56d4dd',
      brightWhite: '#f0f6fc',
    },
    allowProposedApi: true,
  });

  const fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.loadAddon(new Unicode11Addon());

  term.open(container);

  // WebGL renderer for performance (falls back to canvas if unavailable)
  try {
    term.loadAddon(new WebglAddon());
  } catch {
    console.warn('WebGL addon failed to load, using canvas renderer');
  }

  fitAddon.fit();

  return term;
}
```

### 5.2 WebSocket Connection

```typescript
interface TerminalMessage {
  type: 'output' | 'scrollback' | 'scrollback_end' | 'status' | 'error';
  data?: ArrayBuffer;       // Binary terminal data (for output/scrollback)
  status?: string;          // Session status changes
  error?: string;           // Error messages
  progress?: number;        // Scrollback replay progress (0-1)
}

class TerminalSession {
  private ws: WebSocket;
  private term: Terminal;
  private isReplayComplete = false;

  constructor(term: Terminal, sessionId: string) {
    this.term = term;
    // Binary WebSocket for raw terminal data
    this.ws = new WebSocket(
      `wss://${location.host}/ws/terminal/${sessionId}`
    );
    this.ws.binaryType = 'arraybuffer';

    this.ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        // Binary frame = terminal output, write directly to xterm
        this.term.write(new Uint8Array(event.data));
      } else {
        // Text frame = control message (JSON)
        const msg: TerminalMessage = JSON.parse(event.data);
        this.handleControlMessage(msg);
      }
    };

    // Send keystrokes to the server
    this.term.onData((data: string) => {
      if (this.ws.readyState === WebSocket.OPEN && this.isReplayComplete) {
        // Send as binary
        const encoder = new TextEncoder();
        this.ws.send(encoder.encode(data));
      }
    });

    // Send terminal resize events
    this.term.onResize(({ cols, rows }) => {
      if (this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({
          type: 'resize',
          cols,
          rows,
        }));
      }
    });

    this.ws.onclose = () => {
      this.term.write('\r\n\x1b[33m[Connection closed. Session continues running on the remote machine.]\x1b[0m\r\n');
    };
  }

  private handleControlMessage(msg: TerminalMessage) {
    switch (msg.type) {
      case 'scrollback_end':
        this.isReplayComplete = true;
        // Now accepting user input
        break;
      case 'status':
        // Update UI badges
        break;
      case 'error':
        this.term.write(`\r\n\x1b[31m[Error: ${msg.error}]\x1b[0m\r\n`);
        break;
    }
  }

  disconnect() {
    this.ws.close();
  }
}
```

### 5.3 Session Replay Player

For completed sessions, we don't use a WebSocket — we fetch the asciicast file and play it back locally.

```typescript
interface AsciicastHeader {
  version: number;
  width: number;
  height: number;
  timestamp: number;
}

type AsciicastEvent = [number, string, string]; // [time, type, data]

class SessionPlayer {
  private events: AsciicastEvent[] = [];
  private currentIndex = 0;
  private playbackSpeed = 1;
  private isPlaying = false;
  private term: Terminal;
  private rafId: number | null = null;
  private startTime = 0;
  private startOffset = 0;

  // Callback for UI updates (progress bar, time display)
  onProgress?: (current: number, total: number) => void;

  constructor(term: Terminal) {
    this.term = term;
  }

  async load(sessionId: string) {
    const response = await fetch(`/api/v1/sessions/${sessionId}/recording`);
    const text = await response.text();
    const lines = text.trim().split('\n');

    // First line is the header
    const header: AsciicastHeader = JSON.parse(lines[0]);
    this.term.resize(header.width, header.height);

    // Remaining lines are events
    this.events = lines.slice(1).map(line => JSON.parse(line));
  }

  get duration(): number {
    if (this.events.length === 0) return 0;
    return this.events[this.events.length - 1][0];
  }

  play() {
    this.isPlaying = true;
    this.startTime = performance.now();
    this.tick();
  }

  pause() {
    this.isPlaying = false;
    this.startOffset = this.currentTime;
    if (this.rafId) cancelAnimationFrame(this.rafId);
  }

  seek(time: number) {
    // Reset terminal and replay up to the target time
    this.term.reset();
    this.currentIndex = 0;
    this.startOffset = time;

    for (let i = 0; i < this.events.length; i++) {
      const [eventTime, type, data] = this.events[i];
      if (eventTime > time) {
        this.currentIndex = i;
        break;
      }
      if (type === 'o') {
        this.term.write(data);
      }
    }

    this.startTime = performance.now();
    this.onProgress?.(time, this.duration);
  }

  setSpeed(speed: number) {
    this.startOffset = this.currentTime;
    this.startTime = performance.now();
    this.playbackSpeed = speed;
  }

  private get currentTime(): number {
    const elapsed = (performance.now() - this.startTime) / 1000;
    return this.startOffset + elapsed * this.playbackSpeed;
  }

  private tick = () => {
    if (!this.isPlaying) return;

    const now = this.currentTime;

    while (this.currentIndex < this.events.length) {
      const [eventTime, type, data] = this.events[this.currentIndex];
      if (eventTime > now) break;

      if (type === 'o') {
        this.term.write(data);
      }
      this.currentIndex++;
    }

    this.onProgress?.(now, this.duration);

    if (this.currentIndex >= this.events.length) {
      this.isPlaying = false;
      return;
    }

    this.rafId = requestAnimationFrame(this.tick);
  };
}
```

### 5.4 Terminal Tab Management

When multiple sessions are open, they render as tabs (like browser tabs or VS Code terminal tabs).

```typescript
// Zustand store for terminal tabs
interface TerminalTab {
  sessionId: string;
  title: string;          // Machine name + short ID
  status: 'live' | 'replay' | 'disconnected';
  hasUnreadOutput: boolean;
  terminalRef: Terminal | null;
  connectionRef: TerminalSession | SessionPlayer | null;
}

interface TerminalStore {
  tabs: TerminalTab[];
  activeTabId: string | null;
  addTab: (sessionId: string, title: string) => void;
  removeTab: (sessionId: string) => void;
  setActiveTab: (sessionId: string) => void;
  markUnread: (sessionId: string) => void;
}
```

**Key behavior:** When you switch tabs, the previous terminal is not destroyed — it's just hidden (display: none on the container). The WebSocket stays open. This means:
- No reconnection delay when switching back.
- Output keeps accumulating in the hidden terminal.
- A small "unread" dot appears on tabs that received output while you weren't looking.

---

## 6. State Management (Zustand)

### 6.1 Store Architecture

Separate stores for separate concerns. Don't put everything in one god store.

```typescript
// stores/sessions.ts — Active and recent sessions
interface SessionsStore {
  sessions: Map<string, Session>;
  activeSessions: () => Session[];
  fetchSessions: () => Promise<void>;
  updateSession: (id: string, patch: Partial<Session>) => void;
}

// stores/machines.ts — Machine registry and health
interface MachinesStore {
  machines: Map<string, Machine>;
  fetchMachines: () => Promise<void>;
  updateHealth: (machineId: string, health: HealthData) => void;
}

// stores/jobs.ts — Job definitions
interface JobsStore {
  jobs: Map<string, Job>;
  currentJob: Job | null;
  fetchJobs: () => Promise<void>;
  saveJob: (job: Job) => Promise<void>;
  deleteJob: (id: string) => Promise<void>;
}

// stores/runs.ts — Job runs and their step states
interface RunsStore {
  runs: Map<string, Run>;
  activeRuns: () => Run[];
  fetchRuns: (filters?: RunFilters) => Promise<void>;
  updateRunStep: (runId: string, stepId: string, patch: Partial<RunStep>) => void;
}

// stores/terminal.ts — Terminal tab management (from section 5.4)
// stores/ui.ts — UI state (sidebar collapsed, active view, modals, etc.)
interface UIStore {
  sidebarCollapsed: boolean;
  activeView: 'command-center' | 'sessions' | 'jobs' | 'runs' | 'machines';
  commandPaletteOpen: boolean;
  toggleSidebar: () => void;
}

// stores/activity.ts — Real-time activity feed
interface ActivityStore {
  events: ActivityEvent[];
  addEvent: (event: ActivityEvent) => void;
  // Keeps last 200 events in memory, older ones fetched via API
}
```

### 6.2 Real-Time Updates (Event WebSocket)

Separate from terminal WebSockets. One persistent connection for system-wide events:

```typescript
// hooks/useEventStream.ts
function useEventStream() {
  const updateSession = useSessionsStore(s => s.updateSession);
  const updateHealth = useMachinesStore(s => s.updateHealth);
  const updateRunStep = useRunsStore(s => s.updateRunStep);
  const addActivity = useActivityStore(s => s.addEvent);

  useEffect(() => {
    const ws = new WebSocket(`wss://${location.host}/ws/events`);

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);

      switch (msg.type) {
        case 'session.status':
          updateSession(msg.sessionId, { status: msg.status });
          break;
        case 'session.exit':
          updateSession(msg.sessionId, {
            status: msg.exitCode === 0 ? 'completed' : 'failed',
            exitCode: msg.exitCode,
            endedAt: msg.endedAt,
          });
          break;
        case 'machine.health':
          updateHealth(msg.machineId, msg.health);
          break;
        case 'machine.status':
          // connected / disconnected
          break;
        case 'run.step.status':
          updateRunStep(msg.runId, msg.stepId, { status: msg.status });
          break;
        case 'run.status':
          // Update run-level status
          break;
        case 'activity':
          addActivity(msg.event);
          break;
      }
    };

    // Auto-reconnect
    ws.onclose = () => {
      setTimeout(() => useEventStream(), 2000);
    };

    return () => ws.close();
  }, []);
}
```

This event WebSocket is opened once when the app loads and stays open. Every significant state change in the backend is broadcast here. The frontend never needs to poll — all updates are push-based.

---

## 7. Component Architecture

### 7.1 Component Tree

```
<App>
├── <AppShell>
│   ├── <TopBar>
│   │   ├── <Logo />
│   │   ├── <GlobalSearch />          (Cmd+K)
│   │   ├── <CommandPalette />        (Cmd+Shift+P, modal overlay)
│   │   └── <UserMenu />
│   ├── <Sidebar>
│   │   ├── <NavItem to="/" icon={LayoutDashboard} label="Command Center" />
│   │   ├── <NavItem to="/sessions" icon={Terminal} label="Sessions" />
│   │   ├── <NavItem to="/jobs" icon={Workflow} label="Jobs" />
│   │   ├── <NavItem to="/runs" icon={Play} label="Runs" />
│   │   └── <NavItem to="/machines" icon={Server} label="Machines" />
│   ├── <MainContent>
│   │   └── <Routes>
│   │       ├── <CommandCenter />
│   │       ├── <SessionsPage />
│   │       │   └── <TerminalView sessionId={id} />
│   │       ├── <JobsPage />
│   │       │   └── <JobEditor jobId={id} />
│   │       ├── <RunsPage />
│   │       │   └── <RunDetail runId={id} />
│   │       └── <MachinesPage />
│   └── <StatusBar />
└── <EventStreamProvider />           (the real-time WebSocket)
```

### 7.2 Key Shared Components

```
components/
├── terminal/
│   ├── TerminalPanel.tsx          — Wraps xterm.js, handles lifecycle
│   ├── TerminalTabs.tsx           — Tab bar for multiple sessions
│   ├── SessionPlayer.tsx          — Replay controls (play/pause/seek/speed)
│   └── TerminalStatusBar.tsx      — Machine, dimensions, duration
├── dag/
│   ├── DAGCanvas.tsx              — ReactFlow wrapper for job step graph
│   ├── StepNode.tsx               — Custom ReactFlow node for a step
│   └── StepEdge.tsx               — Custom edge with status coloring
├── jobs/
│   ├── StepEditor.tsx             — Form for editing a step's config
│   ├── CronInput.tsx              — Cron expression input + human preview
│   ├── TriggerBuilder.tsx         — Cross-job trigger config
│   └── MachineSelector.tsx        — Dropdown with health info
├── sessions/
│   ├── SessionList.tsx            — Filterable list of sessions
│   ├── SessionCard.tsx            — Single session row
│   └── NewSessionModal.tsx        — Machine + dir picker
├── machines/
│   ├── MachineCard.tsx            — Health bars, session list
│   └── ResourceBar.tsx            — Tiny progress bar (CPU, RAM, disk)
├── shared/
│   ├── StatusBadge.tsx            — Color-coded status indicator
│   ├── TimeAgo.tsx                — "12m ago", "3h ago", auto-updates
│   ├── EmptyState.tsx             — Friendly empty state with action
│   ├── ConfirmDialog.tsx          — "Are you sure?" modal
│   ├── Kbd.tsx                    — Keyboard shortcut display
│   └── CommandPalette.tsx         — Fuzzy search action list
└── layout/
    ├── AppShell.tsx
    ├── TopBar.tsx
    ├── Sidebar.tsx
    └── StatusBar.tsx
```

---

## 8. API Client Layer

Thin wrapper around fetch, typed with the backend's API.

```typescript
// api/client.ts
const BASE = '/api/v1';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, error.message);
  }

  return res.json();
}

// api/sessions.ts
export const sessionsApi = {
  list: (filters?: SessionFilters) =>
    request<Session[]>(`/sessions?${toQuery(filters)}`),
  get: (id: string) =>
    request<Session>(`/sessions/${id}`),
  create: (params: CreateSessionParams) =>
    request<Session>('/sessions', { method: 'POST', body: JSON.stringify(params) }),
  kill: (id: string) =>
    request<void>(`/sessions/${id}`, { method: 'DELETE' }),
  recording: (id: string) =>
    fetch(`${BASE}/sessions/${id}/recording`).then(r => r.text()),
};

// api/jobs.ts
export const jobsApi = {
  list: () => request<Job[]>('/jobs'),
  get: (id: string) => request<JobDetail>(`/jobs/${id}`),
  create: (params: CreateJobParams) =>
    request<Job>('/jobs', { method: 'POST', body: JSON.stringify(params) }),
  update: (id: string, params: UpdateJobParams) =>
    request<Job>(`/jobs/${id}`, { method: 'PUT', body: JSON.stringify(params) }),
  delete: (id: string) =>
    request<void>(`/jobs/${id}`, { method: 'DELETE' }),
  run: (id: string) =>
    request<Run>(`/jobs/${id}/runs`, { method: 'POST', body: JSON.stringify({ trigger_type: 'manual' }) }),
  // Steps
  addStep: (jobId: string, params: CreateStepParams) =>
    request<Step>(`/jobs/${jobId}/steps`, { method: 'POST', body: JSON.stringify(params) }),
  updateStep: (jobId: string, stepId: string, params: UpdateStepParams) =>
    request<Step>(`/jobs/${jobId}/steps/${stepId}`, { method: 'PUT', body: JSON.stringify(params) }),
  deleteStep: (jobId: string, stepId: string) =>
    request<void>(`/jobs/${jobId}/steps/${stepId}`, { method: 'DELETE' }),
  // Schedules
  addSchedule: (jobId: string, params: CreateScheduleParams) =>
    request<Schedule>(`/jobs/${jobId}/schedules`, { method: 'POST', body: JSON.stringify(params) }),
  // Triggers
  addTrigger: (jobId: string, params: CreateTriggerParams) =>
    request<Trigger>(`/jobs/${jobId}/triggers`, { method: 'POST', body: JSON.stringify(params) }),
};

// api/runs.ts, api/machines.ts follow the same pattern
```

---

## 9. Routing Structure

```typescript
// App.tsx
import { BrowserRouter, Routes, Route } from 'react-router-dom';

function App() {
  return (
    <BrowserRouter>
      <EventStreamProvider>
        <AppShell>
          <Routes>
            <Route path="/" element={<CommandCenter />} />

            <Route path="/sessions" element={<SessionsPage />} />
            <Route path="/sessions/:sessionId" element={<TerminalView />} />

            <Route path="/jobs" element={<JobsPage />} />
            <Route path="/jobs/new" element={<JobEditor />} />
            <Route path="/jobs/:jobId" element={<JobDetail />} />
            <Route path="/jobs/:jobId/edit" element={<JobEditor />} />

            <Route path="/runs" element={<RunsPage />} />
            <Route path="/runs/:runId" element={<RunDetail />} />

            <Route path="/machines" element={<MachinesPage />} />
            <Route path="/machines/:machineId" element={<MachineDetail />} />

            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/settings/credentials" element={<CredentialsPage />} />
          </Routes>
        </AppShell>
      </EventStreamProvider>
    </BrowserRouter>
  );
}
```

---

## 10. Design System

### 10.1 Color Palette

```css
:root {
  /* Base — dark theme, optimized for terminal-heavy use */
  --bg-primary: #0d1117;         /* Main background (matches terminal) */
  --bg-secondary: #161b22;       /* Cards, sidebar */
  --bg-tertiary: #21262d;        /* Hover states, active items */
  --bg-overlay: #30363db3;       /* Modal backdrop */

  --text-primary: #c9d1d9;       /* Main text */
  --text-secondary: #8b949e;     /* Muted text, labels */
  --text-tertiary: #6e7681;      /* Disabled, timestamps */

  --border-primary: #30363d;     /* Card borders, dividers */
  --border-active: #58a6ff;      /* Focused inputs, active tabs */

  /* Status colors */
  --status-running: #58a6ff;     /* Blue — in progress */
  --status-success: #7ee787;     /* Green — completed */
  --status-error: #ff7b72;       /* Red — failed */
  --status-warning: #d29922;     /* Amber — degraded, draining */
  --status-pending: #6e7681;     /* Gray — waiting */
  --status-attached: #bc8cff;    /* Purple — user is attached to session */

  /* Accents */
  --accent-primary: #58a6ff;     /* Links, primary buttons */
  --accent-hover: #79c0ff;       /* Hover on accent elements */

  /* Surfaces */
  --surface-card: #161b22;
  --surface-input: #0d1117;
  --surface-tooltip: #1f2937;
}
```

**Why dark-first:** This is a terminal tool. Users will stare at terminal output for hours. A light theme surrounding a dark terminal creates visual jarring. Dark everywhere, consistent contrast, zero eye strain.

**Light theme:** Optional toggle in settings. Swap CSS variables. Not a priority for V1 — build it dark, add light later.

### 10.2 Typography

```css
:root {
  /* UI text */
  --font-sans: 'IBM Plex Sans', -apple-system, BlinkMacSystemFont, sans-serif;

  /* Terminal & code */
  --font-mono: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;

  /* Scale */
  --text-xs: 0.75rem;     /* 12px — timestamps, badges */
  --text-sm: 0.8125rem;   /* 13px — secondary labels */
  --text-base: 0.875rem;  /* 14px — body text (dense UI needs smaller base) */
  --text-lg: 1rem;        /* 16px — section headers */
  --text-xl: 1.25rem;     /* 20px — page titles */
  --text-2xl: 1.5rem;     /* 24px — hero numbers (session count, etc.) */
}
```

**IBM Plex Sans** — clean, technical, slightly industrial. Not generic (not Inter, not Roboto), not flashy. Professional tool aesthetic.

**JetBrains Mono** — the terminal font. Ligatures optional (some people love them, some hate them — make it a setting).

**14px base** — deliberate. This is a dense, information-heavy UI. 16px wastes space. Every pixel matters when you're showing terminals + DAG graphs + session lists.

### 10.3 Spacing & Layout

```css
:root {
  --space-1: 0.25rem;   /* 4px */
  --space-2: 0.5rem;    /* 8px */
  --space-3: 0.75rem;   /* 12px */
  --space-4: 1rem;      /* 16px */
  --space-6: 1.5rem;    /* 24px */
  --space-8: 2rem;      /* 32px */

  --radius-sm: 4px;     /* Buttons, badges */
  --radius-md: 6px;     /* Cards, inputs */
  --radius-lg: 8px;     /* Modals, larger surfaces */

  --sidebar-expanded: 220px;
  --sidebar-collapsed: 56px;
  --topbar-height: 48px;
  --statusbar-height: 32px;
}
```

Tight spacing. This isn't a marketing site — it's a tool. Dense but readable.

### 10.4 Status Badge Component

The most-used component in the entire app:

```tsx
type Status = 'running' | 'completed' | 'failed' | 'pending' | 'connected' |
              'disconnected' | 'attached' | 'detached' | 'draining' | 'killed' |
              'waiting' | 'skipped' | 'cancelled' | 'starting' | 'unknown';

const statusConfig: Record<Status, { color: string; icon: LucideIcon; label: string }> = {
  running:      { color: 'var(--status-running)',  icon: Loader2,    label: 'Running' },
  completed:    { color: 'var(--status-success)',  icon: CheckCircle, label: 'Completed' },
  failed:       { color: 'var(--status-error)',    icon: XCircle,    label: 'Failed' },
  pending:      { color: 'var(--status-pending)',  icon: Circle,     label: 'Pending' },
  connected:    { color: 'var(--status-success)',  icon: Wifi,       label: 'Connected' },
  disconnected: { color: 'var(--status-error)',    icon: WifiOff,    label: 'Disconnected' },
  attached:     { color: 'var(--status-attached)', icon: Eye,        label: 'Attached' },
  detached:     { color: 'var(--status-warning)',  icon: EyeOff,     label: 'Detached' },
  starting:     { color: 'var(--status-warning)',  icon: Loader2,    label: 'Starting' },
  waiting:      { color: 'var(--status-pending)',  icon: Clock,      label: 'Waiting' },
  draining:     { color: 'var(--status-warning)',  icon: AlertTriangle, label: 'Draining' },
  killed:       { color: 'var(--status-error)',    icon: Slash,      label: 'Killed' },
  skipped:      { color: 'var(--status-pending)',  icon: SkipForward, label: 'Skipped' },
  cancelled:    { color: 'var(--status-warning)',  icon: Ban,        label: 'Cancelled' },
  unknown:      { color: 'var(--status-pending)',  icon: HelpCircle, label: 'Unknown' },
};

function StatusBadge({ status, showLabel = true }: { status: Status; showLabel?: boolean }) {
  const config = statusConfig[status];
  const Icon = config.icon;
  const isAnimated = status === 'running' || status === 'starting';

  return (
    <span className="inline-flex items-center gap-1.5">
      <Icon
        size={14}
        style={{ color: config.color }}
        className={isAnimated ? 'animate-spin' : ''}
      />
      {showLabel && (
        <span className="text-sm" style={{ color: config.color }}>
          {config.label}
        </span>
      )}
    </span>
  );
}
```

---

## 11. Frontend Project Structure

```
frontend/
├── public/
│   └── favicon.svg
├── src/
│   ├── main.tsx                   — Entry point
│   ├── App.tsx                    — Router + providers
│   │
│   ├── api/                       — API client layer
│   │   ├── client.ts              — Base fetch wrapper
│   │   ├── sessions.ts
│   │   ├── jobs.ts
│   │   ├── runs.ts
│   │   └── machines.ts
│   │
│   ├── stores/                    — Zustand stores
│   │   ├── sessions.ts
│   │   ├── machines.ts
│   │   ├── jobs.ts
│   │   ├── runs.ts
│   │   ├── terminal.ts
│   │   ├── activity.ts
│   │   └── ui.ts
│   │
│   ├── hooks/                     — Custom hooks
│   │   ├── useEventStream.ts      — System-wide WebSocket
│   │   ├── useTerminalSession.ts  — Terminal WebSocket lifecycle
│   │   ├── useSessionPlayer.ts    — Replay controls
│   │   ├── useKeyboardShortcuts.ts
│   │   └── useResponsive.ts      — Breakpoint detection
│   │
│   ├── views/                     — Page-level components (one per route)
│   │   ├── CommandCenter.tsx
│   │   ├── SessionsPage.tsx
│   │   ├── TerminalView.tsx
│   │   ├── JobsPage.tsx
│   │   ├── JobEditor.tsx
│   │   ├── RunsPage.tsx
│   │   ├── RunDetail.tsx
│   │   ├── MachinesPage.tsx
│   │   └── SettingsPage.tsx
│   │
│   ├── components/                — Reusable components (from section 7.2)
│   │   ├── terminal/
│   │   ├── dag/
│   │   ├── jobs/
│   │   ├── sessions/
│   │   ├── machines/
│   │   ├── shared/
│   │   └── layout/
│   │
│   ├── lib/                       — Utilities
│   │   ├── terminal.ts            — xterm.js setup + config
│   │   ├── player.ts              — Asciicast replay engine
│   │   ├── cron.ts                — Cron expression parser + human-readable
│   │   ├── format.ts              — Date, duration, byte formatting
│   │   └── types.ts               — Shared TypeScript types
│   │
│   └── styles/
│       ├── globals.css            — CSS variables, base styles
│       └── terminal.css           — xterm.js theme overrides
│
├── index.html
├── vite.config.ts
├── tailwind.config.ts
├── tsconfig.json
└── package.json
```

---

## 12. Build & Integration with Go Server

The frontend is a static build that the Go server serves.

```typescript
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../frontend/dist',     // Output into the Go project
    emptyOutDir: true,
  },
  server: {
    port: 3000,
    proxy: {
      '/api': 'https://localhost:8443',
      '/ws': {
        target: 'wss://localhost:8443',
        ws: true,
      },
    },
  },
});
```

**Development:** `npm run dev` starts Vite on port 3000, proxies API/WebSocket to the Go server on 8443. Hot module reload, fast iteration.

**Production:** `npm run build` produces static files in `frontend/dist/`. The Go server embeds them with `embed.FS`:

```go
//go:embed frontend/dist/*
var frontendFS embed.FS

func (s *Server) serveFrontend() http.Handler {
    stripped, _ := fs.Sub(frontendFS, "frontend/dist")
    fileServer := http.FileServer(http.FS(stripped))

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Try to serve static file; fall back to index.html for SPA routing
        path := r.URL.Path
        if _, err := fs.Stat(stripped, strings.TrimPrefix(path, "/")); err != nil {
            // Not a static file — serve index.html (SPA client-side routing)
            r.URL.Path = "/"
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

**Single binary ships everything.** `claude-plane-server` contains the compiled frontend. No separate deployment, no nginx, no CDN.

---

## 13. Accessibility & Performance Notes

### Accessibility
- All interactive elements are keyboard-focusable (tab order).
- Status colors always paired with icons (not color-only — colorblind safe).
- ARIA labels on icon-only buttons.
- Terminal component traps focus when active (keystrokes go to terminal, not UI).
- Escape exits terminal focus, returns to UI navigation.

### Performance
- **xterm.js WebGL renderer** — handles massive output without lag (build logs, large diffs).
- **Virtualized lists** — session list, run list, activity feed use `@tanstack/react-virtual` when > 100 items.
- **Lazy route loading** — `React.lazy()` for JobEditor, RunDetail, SettingsPage (they're heavy, no need to load on first paint).
- **WebSocket binary frames** — terminal data sent as ArrayBuffer, not base64-encoded text. Minimal overhead.
- **Debounced resize** — terminal resize events debounced (100ms) to avoid flooding the server with resize commands during window drag.

---

## Appendix: Feature Matrix by Device

| Feature | Desktop | Laptop | Tablet | Phone |
|---------|---------|--------|--------|-------|
| Command center dashboard | Full | Full | Simplified | Simplified |
| Live terminal (type into session) | Yes | Yes | Yes (with on-screen keyboard) | No (view-only) |
| Session replay | Full controls | Full controls | Play/pause only | Play/pause only |
| Job editor (DAG builder) | Full (side-by-side) | Full (stacked) | View-only | Not available |
| Run detail (DAG + terminal) | Full | Full | Stacked, one at a time | Status only |
| New session creation | Full modal | Full modal | Full modal | Not available |
| Kill session | Yes | Yes | Yes | Yes |
| Machine health | Full cards | Compact list | Compact list | Status dots only |
| Keyboard shortcuts | Full | Full | N/A | N/A |
| Command palette | Yes | Yes | No | No |
