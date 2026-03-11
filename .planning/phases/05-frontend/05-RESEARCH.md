# Phase 5: Frontend - Research

**Researched:** 2026-03-12
**Domain:** React SPA -- dashboard, session management, machine status UI
**Confidence:** HIGH

## Summary

Phase 5 builds the full React SPA that gives users cross-fleet visibility and session lifecycle controls. It depends on Phase 4 which already delivers: (1) the xterm.js terminal component, (2) the WebSocket hook for terminal I/O, and (3) the REST API handlers for sessions and machines. Phase 5's job is to build the application shell (layout, navigation, routing), the Command Center dashboard, the Sessions list view, the Machines view, and wire everything together with state management and real-time event updates.

The single explicit requirement is SESS-04 (list all active sessions across all machines), but the success criteria expand this to include machine online/offline status, session lifecycle actions via UI, and embedding the frontend in the server binary. The design document (`docs/internal/product/frontend_v1.md`) provides exhaustive specifications for component hierarchy, state management, routing, design system, and API client patterns. This research validates and updates the tech stack versions and identifies implementation patterns.

**Primary recommendation:** Use React 19 (not 18), Vite 7, Tailwind CSS v4, xterm.js v6 (@xterm/xterm), React Router v7, Zustand 5, TanStack Query v5, and Sonner 2. Follow the project structure and component architecture defined in the frontend design doc with only the version/API updates noted below.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SESS-04 | User can list all active sessions across all machines | Sessions list view with filters, Command Center active sessions panel, backed by `GET /api/v1/sessions` REST endpoint and real-time WebSocket event stream |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | 19.x | UI framework | Current stable, design doc says 18+ but 19 is current and all deps support it |
| Vite | 7.x | Build tool / dev server | Current stable, fast HMR, native Tailwind v4 plugin support |
| React Router | 7.x | Client-side routing | Current stable, simplified imports (no separate react-router-dom) |
| Zustand | 5.x | Client state management | Minimal boilerplate, excellent TS support, standard for client state in 2026 |
| TanStack Query | 5.x (@tanstack/react-query) | Server state / data fetching | Caching, refetching, loading states for REST API data |
| @xterm/xterm | 6.x | Terminal emulator | Scoped package (replaces old `xterm` package), 30% smaller bundle |
| @xterm/addon-fit | 6.x | Terminal auto-resize | Pairs with @xterm/xterm |
| @xterm/addon-webgl | 6.x | WebGL terminal renderer | Performance for large output |
| Tailwind CSS | 4.x | Utility-first CSS | CSS-first config (no tailwind.config.js), Vite plugin integration |
| @tailwindcss/vite | 4.x | Vite plugin for Tailwind | First-party plugin, replaces PostCSS setup |
| TypeScript | 5.x | Type safety | Project standard |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| lucide-react | latest | Icons | All UI icons, tree-shakeable |
| sonner | 2.x | Toast notifications | Session created/killed, errors, connection status changes |
| reconnecting-websocket | 4.x | WebSocket auto-reconnect | Event stream WebSocket (not terminal -- terminal uses raw WS) |
| @tanstack/react-virtual | 3.x | List virtualization | Session list, activity feed when >100 items |
| date-fns | 3.x | Date formatting | "12m ago", duration formatting |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| React 19 | React 18 | Design doc says 18+; all deps support 19, use current |
| Tailwind v4 | Tailwind v3 | v4 is current, simpler setup with Vite plugin, no config file needed |
| React Router v7 | React Router v6 | v7 is current stable, simplified imports, v6 is legacy |
| TanStack Query | Plain fetch + Zustand | TanStack Query handles caching/refetch/loading states -- don't hand-roll |

### Version Update Notes from Design Doc

The frontend design doc specifies slightly older versions. Key updates:

| Design Doc Says | Current (March 2026) | Impact |
|-----------------|---------------------|--------|
| React 18+ | React 19.2.x | Minor API differences; hooks work the same |
| xterm.js (unscoped) | @xterm/xterm 6.x (scoped) | Import paths change: `xterm` -> `@xterm/xterm`, `xterm-addon-fit` -> `@xterm/addon-fit` |
| React Router v6 | React Router v7.13.x | Import from `react-router` (not `react-router-dom`), API is compatible |
| Tailwind CSS (config file) | Tailwind CSS v4 (CSS-first) | No `tailwind.config.ts`; use `@theme` in CSS + `@tailwindcss/vite` plugin |
| Zustand (unversioned) | Zustand 5.x | API is stable, store creation slightly different |

**Installation:**
```bash
# Inside web/ directory
npm create vite@latest . -- --template react-ts

# Core
npm install react react-dom react-router @tanstack/react-query zustand

# Terminal
npm install @xterm/xterm @xterm/addon-fit @xterm/addon-webgl @xterm/addon-unicode11

# UI
npm install tailwindcss @tailwindcss/vite lucide-react sonner date-fns reconnecting-websocket @tanstack/react-virtual

# Dev
npm install -D typescript @types/react @types/react-dom eslint prettier vitest @vitejs/plugin-react
```

## Architecture Patterns

### Project Structure

Follow the design doc structure (section 11), placed in `web/`:

```
web/
├── public/
│   └── favicon.svg
├── src/
│   ├── main.tsx                   # Entry point
│   ├── App.tsx                    # Router + providers
│   ├── api/                       # API client layer (typed fetch wrappers)
│   │   ├── client.ts              # Base request() helper
│   │   ├── sessions.ts
│   │   └── machines.ts
│   ├── stores/                    # Zustand stores (client state only)
│   │   ├── sessions.ts
│   │   ├── machines.ts
│   │   ├── terminal.ts
│   │   ├── activity.ts
│   │   └── ui.ts
│   ├── hooks/                     # Custom hooks
│   │   ├── useEventStream.ts      # System-wide event WebSocket
│   │   ├── useTerminalSession.ts  # Terminal WebSocket lifecycle (from Phase 4)
│   │   └── useKeyboardShortcuts.ts
│   ├── views/                     # Page-level components
│   │   ├── CommandCenter.tsx
│   │   ├── SessionsPage.tsx
│   │   ├── TerminalView.tsx       # (from Phase 4, enhanced)
│   │   └── MachinesPage.tsx
│   ├── components/                # Reusable components
│   │   ├── terminal/              # (from Phase 4)
│   │   ├── sessions/
│   │   │   ├── SessionList.tsx
│   │   │   ├── SessionCard.tsx
│   │   │   └── NewSessionModal.tsx
│   │   ├── machines/
│   │   │   ├── MachineCard.tsx
│   │   │   └── ResourceBar.tsx
│   │   ├── shared/
│   │   │   ├── StatusBadge.tsx
│   │   │   ├── TimeAgo.tsx
│   │   │   ├── EmptyState.tsx
│   │   │   └── ConfirmDialog.tsx
│   │   └── layout/
│   │       ├── AppShell.tsx
│   │       ├── TopBar.tsx
│   │       ├── Sidebar.tsx
│   │       └── StatusBar.tsx
│   ├── lib/                       # Utilities
│   │   ├── format.ts              # Date, duration formatting
│   │   └── types.ts               # Shared TypeScript types
│   └── styles/
│       ├── globals.css            # CSS variables, @import "tailwindcss", @theme
│       └── terminal.css           # xterm.js overrides
├── index.html
├── vite.config.ts
└── tsconfig.json
```

### Pattern 1: Server State vs Client State Separation

**What:** TanStack Query owns all server-fetched data (sessions list, machines list). Zustand owns UI state (sidebar collapsed, active view, terminal tabs). Never duplicate server data in Zustand.

**When to use:** Always. This is the foundational state pattern.

**Example:**
```typescript
// Use TanStack Query for server data
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { sessionsApi } from '../api/sessions';

export function useSessions(filters?: SessionFilters) {
  return useQuery({
    queryKey: ['sessions', filters],
    queryFn: () => sessionsApi.list(filters),
    refetchInterval: 30_000, // Fallback polling (WebSocket is primary)
  });
}

export function useCreateSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: sessionsApi.create,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
  });
}

// Use Zustand for client-only state
import { create } from 'zustand';

interface UIStore {
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
}

export const useUIStore = create<UIStore>((set) => ({
  sidebarCollapsed: false,
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
}));
```

### Pattern 2: Real-Time WebSocket Event Stream + Query Invalidation

**What:** A single persistent WebSocket (`/ws/events`) receives push events for session status changes, machine health, etc. On receiving an event, invalidate the relevant TanStack Query cache so the UI re-fetches fresh data.

**When to use:** For all real-time state updates (machine connected/disconnected, session created/exited).

**Example:**
```typescript
// hooks/useEventStream.ts
import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import ReconnectingWebSocket from 'reconnecting-websocket';

export function useEventStream() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const ws = new ReconnectingWebSocket(
      `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws/events`
    );

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      switch (msg.type) {
        case 'session.status':
        case 'session.exit':
          queryClient.invalidateQueries({ queryKey: ['sessions'] });
          break;
        case 'machine.status':
        case 'machine.health':
          queryClient.invalidateQueries({ queryKey: ['machines'] });
          break;
      }
    };

    return () => ws.close();
  }, [queryClient]);
}
```

### Pattern 3: Vite Config with Tailwind v4 + Dev Proxy

**What:** Vite config uses the @tailwindcss/vite plugin (no PostCSS), proxies API/WS to Go server.

```typescript
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../internal/server/frontend/dist',
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

### Pattern 4: Tailwind v4 CSS-First Theming

**What:** Tailwind v4 replaces `tailwind.config.js` with CSS `@theme` directives.

```css
/* styles/globals.css */
@import "tailwindcss";

@theme {
  --color-bg-primary: #0d1117;
  --color-bg-secondary: #161b22;
  --color-bg-tertiary: #21262d;
  --color-text-primary: #c9d1d9;
  --color-text-secondary: #8b949e;
  --color-status-running: #58a6ff;
  --color-status-success: #7ee787;
  --color-status-error: #ff7b72;
  --color-status-warning: #d29922;
  --color-status-pending: #6e7681;
  --color-accent-primary: #58a6ff;
  --font-sans: 'IBM Plex Sans', -apple-system, BlinkMacSystemFont, sans-serif;
  --font-mono: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;
}
```

### Pattern 5: Go Embed for SPA Serving

**What:** Production build output is embedded into the server binary via `go:embed`.

```go
//go:embed frontend/dist/*
var frontendFS embed.FS

func serveFrontend() http.Handler {
    stripped, _ := fs.Sub(frontendFS, "frontend/dist")
    fileServer := http.FileServer(http.FS(stripped))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        path := strings.TrimPrefix(r.URL.Path, "/")
        if _, err := fs.Stat(stripped, path); err != nil {
            r.URL.Path = "/"
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

### Anti-Patterns to Avoid

- **Storing server data in Zustand:** Use TanStack Query for anything fetched from `/api/v1/*`. Zustand is for UI-only state (sidebar, modals, tabs).
- **Multiple WebSocket connections for events:** Use ONE event stream WebSocket + TanStack Query invalidation. Terminal WebSockets are separate (one per attached session).
- **Polling instead of WebSocket push:** The event WebSocket pushes all state changes. TanStack Query's `refetchInterval` is a fallback, not the primary mechanism.
- **Destroying terminal instances on tab switch:** Hide terminals with `display: none`, don't unmount. WebSocket stays open, output accumulates.
- **Using tailwind.config.js with v4:** Tailwind v4 uses CSS-first configuration. No JS config file.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Data fetching + caching | Custom fetch + useState | TanStack Query | Handles loading, error, cache, refetch, optimistic updates |
| WebSocket reconnection | Custom reconnect logic | reconnecting-websocket | Exponential backoff, buffering, event-based |
| Toast notifications | Custom toast system | Sonner | Accessible, animated, stacking, auto-dismiss |
| List virtualization | Custom scroll handler | @tanstack/react-virtual | Efficient DOM recycling for long lists |
| Terminal emulation | Canvas/DOM rendering | @xterm/xterm + @xterm/addon-webgl | Battle-tested, handles escape sequences, WebGL performance |
| Time formatting | Custom "ago" functions | date-fns formatDistanceToNow | Edge cases (timezone, locale, boundaries) handled |

**Key insight:** This is a dashboard + terminal app. The UI patterns (lists, cards, status badges, modals) are standard. The complex part is terminal integration, which Phase 4 already handles. Phase 5 is about building standard CRUD views and wiring them to REST APIs.

## Common Pitfalls

### Pitfall 1: xterm.js v6 Scoped Package Imports
**What goes wrong:** Code uses old `import { Terminal } from 'xterm'` syntax.
**Why it happens:** Design doc examples use old unscoped imports.
**How to avoid:** All xterm imports must use scoped packages: `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-webgl`, `@xterm/addon-unicode11`.
**Warning signs:** Build errors about missing `xterm` module.

### Pitfall 2: Tailwind v4 Configuration Migration
**What goes wrong:** Creating `tailwind.config.ts` which is ignored by v4.
**Why it happens:** Most tutorials and the design doc reference v3 patterns.
**How to avoid:** Use `@import "tailwindcss"` in CSS + `@theme` for custom values. Use `@tailwindcss/vite` plugin. No JS config file needed.
**Warning signs:** Custom colors/fonts not applying despite being in a config file.

### Pitfall 3: React Router v7 Import Changes
**What goes wrong:** Importing from `react-router-dom` which is now unnecessary.
**Why it happens:** Design doc uses v6 patterns.
**How to avoid:** Import everything from `react-router`. The `react-router-dom` package still exists but is no longer needed as a separate install.
**Warning signs:** Having both `react-router` and `react-router-dom` in package.json.

### Pitfall 4: WebSocket Authentication on Upgrade
**What goes wrong:** WebSocket connections fail because auth token isn't sent on upgrade.
**Why it happens:** WebSocket API doesn't support custom headers in the browser.
**How to avoid:** Send JWT as a query parameter on the WebSocket URL (`/ws/events?token=xxx`) or use cookie-based auth. The server must check auth during the HTTP upgrade handshake.
**Warning signs:** 401 on WebSocket connection despite being logged in.

### Pitfall 5: SPA Routing with go:embed
**What goes wrong:** Direct URL navigation to `/sessions/abc` returns 404.
**Why it happens:** Go file server looks for a literal file `sessions/abc` which doesn't exist.
**How to avoid:** The SPA fallback handler must serve `index.html` for any path that doesn't match a static file. See the Go embed pattern above.
**Warning signs:** Refresh on any non-root URL returns 404.

### Pitfall 6: Terminal Focus Trapping
**What goes wrong:** Keyboard shortcuts (Cmd+K, Cmd+N) don't work when terminal is focused.
**Why it happens:** xterm.js captures all keystrokes when focused.
**How to avoid:** Use `term.attachCustomKeyEventHandler()` to intercept app-level shortcuts before xterm processes them. Let Escape blur the terminal and return focus to the app.
**Warning signs:** Can't use any keyboard shortcut while in a terminal.

## Code Examples

### xterm.js v6 Setup (Updated from Design Doc)
```typescript
// Source: @xterm/xterm v6 scoped packages
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import '@xterm/xterm/css/xterm.css';

function createTerminal(container: HTMLElement): Terminal {
  const term = new Terminal({
    cursorBlink: true,
    cursorStyle: 'block',
    fontSize: 14,
    fontFamily: '"JetBrains Mono", "Fira Code", monospace',
    lineHeight: 1.2,
    theme: {
      background: '#0d1117',
      foreground: '#c9d1d9',
      cursor: '#58a6ff',
      selectionBackground: '#264f78',
    },
  });

  const fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.loadAddon(new Unicode11Addon());
  term.open(container);

  try {
    term.loadAddon(new WebglAddon());
  } catch {
    console.warn('WebGL unavailable, using canvas renderer');
  }

  fitAddon.fit();
  return term;
}
```

### React Router v7 App Setup
```typescript
// App.tsx -- React Router v7 (import from 'react-router')
import { BrowserRouter, Routes, Route } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 10_000 },
  },
});

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppShell>
          <Routes>
            <Route path="/" element={<CommandCenter />} />
            <Route path="/sessions" element={<SessionsPage />} />
            <Route path="/sessions/:sessionId" element={<TerminalView />} />
            <Route path="/machines" element={<MachinesPage />} />
          </Routes>
        </AppShell>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
```

### API Client Pattern (from Design Doc)
```typescript
// api/client.ts
const BASE = '/api/v1';

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const token = localStorage.getItem('token');
  const res = await fetch(`${BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
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
    request<Session[]>(`/sessions${filters ? `?${new URLSearchParams(filters as any)}` : ''}`),
  get: (id: string) => request<Session>(`/sessions/${id}`),
  create: (params: CreateSessionParams) =>
    request<Session>('/sessions', { method: 'POST', body: JSON.stringify(params) }),
  kill: (id: string) =>
    request<void>(`/sessions/${id}`, { method: 'DELETE' }),
};

// api/machines.ts
export const machinesApi = {
  list: () => request<Machine[]>('/machines'),
  get: (id: string) => request<Machine>(`/machines/${id}`),
};
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `import 'xterm'` | `import '@xterm/xterm'` | xterm.js v5.4+ | All imports need scoped package names |
| tailwind.config.js | CSS @theme directive | Tailwind CSS v4 (Jan 2025) | No JS config, CSS-first theming |
| react-router-dom | react-router (single package) | React Router v7 (late 2024) | Simplified imports |
| React 18 | React 19 | Dec 2024 | New hooks, compiler ready, compatible with existing code |
| PostCSS for Tailwind | @tailwindcss/vite plugin | Tailwind CSS v4 | Faster builds, simpler config |

**Deprecated/outdated:**
- `xterm` (unscoped npm package) -- use `@xterm/xterm`
- `xterm-addon-fit` -- use `@xterm/addon-fit`
- `tailwind.config.js` -- use CSS `@theme` in Tailwind v4
- `react-router-dom` as separate package -- import from `react-router`

## Open Questions

1. **Phase 4 Frontend Foundation Scope**
   - What we know: Phase 4 plan 04-03 creates "Frontend xterm.js terminal component + WebSocket hook". This likely sets up the Vite project, basic xterm component, and WS connection.
   - What's unclear: Exactly what project scaffold Phase 4 leaves behind (package.json? routing? layout?).
   - Recommendation: Phase 5 Plan 1 should start by assessing what Phase 4 built and extending it. If Phase 4 created a minimal scaffold, Phase 5 adds the full app shell. If Phase 4 only created components without a scaffold, Phase 5 must set up the full Vite project.

2. **Event WebSocket Authentication**
   - What we know: REST uses JWT Bearer token. WebSocket API in browsers doesn't support custom headers.
   - What's unclear: Whether Phase 3/4 already established a pattern for WebSocket auth (query param? cookie?).
   - Recommendation: Use query parameter token (`/ws/events?token=xxx`). Server validates on upgrade.

3. **go:embed Build Path**
   - What we know: Design doc says `outDir: '../frontend/dist'`. CLAUDE.md says `web/` for frontend.
   - What's unclear: Exact embed path the server uses.
   - Recommendation: Build to `web/dist/`, embed from there. Align with whatever Phase 4 established.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Vitest (latest, paired with Vite 7) |
| Config file | `web/vite.config.ts` (Vitest uses Vite config) |
| Quick run command | `cd web && npx vitest run --reporter=verbose` |
| Full suite command | `cd web && npx vitest run` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SESS-04 | Sessions list displays all active sessions across machines | unit | `cd web && npx vitest run src/__tests__/views/SessionsPage.test.tsx -x` | Wave 0 |
| SESS-04 | Command Center shows active sessions panel | unit | `cd web && npx vitest run src/__tests__/views/CommandCenter.test.tsx -x` | Wave 0 |
| SC-2 | Machine list shows online/offline status | unit | `cd web && npx vitest run src/__tests__/views/MachinesPage.test.tsx -x` | Wave 0 |
| SC-3 | Session lifecycle actions accessible via UI | unit | `cd web && npx vitest run src/__tests__/components/sessions/NewSessionModal.test.tsx -x` | Wave 0 |
| SC-4 | Frontend embedded in server binary | smoke | `cd web && npm run build && test -f dist/index.html` | Wave 0 |

### Sampling Rate
- **Per task commit:** `cd web && npx vitest run --reporter=verbose`
- **Per wave merge:** `cd web && npx vitest run && npm run build`
- **Phase gate:** Full suite green + production build succeeds

### Wave 0 Gaps
- [ ] `web/src/__tests__/views/SessionsPage.test.tsx` -- covers SESS-04 session listing
- [ ] `web/src/__tests__/views/CommandCenter.test.tsx` -- covers SESS-04 dashboard panel
- [ ] `web/src/__tests__/views/MachinesPage.test.tsx` -- covers SC-2 machine status
- [ ] `web/src/__tests__/components/sessions/NewSessionModal.test.tsx` -- covers SC-3 lifecycle
- [ ] `web/src/__tests__/setup.ts` -- test setup with React Testing Library
- [ ] Vitest config in `web/vite.config.ts` -- test environment: jsdom
- [ ] Dev dependencies: `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom`

## Sources

### Primary (HIGH confidence)
- `docs/internal/product/frontend_v1.md` -- project frontend design document (component hierarchy, state management, routing, design system)
- `docs/internal/product/backend_v1.md` -- REST API endpoints, WebSocket protocol, Go embed pattern
- `CLAUDE.md` -- tech stack decisions, project structure

### Secondary (MEDIUM confidence)
- [React versions](https://react.dev/versions) -- React 19.2.x is current stable
- [Vite releases](https://vite.dev/releases) -- Vite 7.3.x is current stable
- [React Router npm](https://www.npmjs.com/package/react-router) -- v7.13.x, import from `react-router`
- [xterm.js releases](https://github.com/xtermjs/xterm.js/releases) -- v6.0.0, scoped @xterm/* packages
- [Tailwind CSS v4 blog](https://tailwindcss.com/blog/tailwindcss-v4) -- CSS-first config, @tailwindcss/vite plugin
- [Zustand npm](https://www.npmjs.com/package/zustand) -- v5.0.x
- [TanStack Query npm](https://www.npmjs.com/package/@tanstack/react-query) -- v5.90.x
- [Sonner npm](https://www.npmjs.com/package/sonner) -- v2.0.x

### Tertiary (LOW confidence)
- None -- all findings verified with multiple sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all versions verified on npm/official docs, compatibility confirmed
- Architecture: HIGH -- design doc is comprehensive, patterns validated against current ecosystem
- Pitfalls: HIGH -- version migration issues are well-documented across official changelogs
- Validation: MEDIUM -- test structure is standard but depends on Phase 4 scaffold state

**Research date:** 2026-03-12
**Valid until:** 2026-04-12 (stable ecosystem, 30-day validity)
