# Multi-View Terminal Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a multi-view page that displays 2-6 terminal sessions simultaneously in configurable, resizable grid layouts with saved workspaces.

**Architecture:** Frontend-only feature. Each terminal pane runs its own WebSocket via the existing `useTerminalSession` hook. Layout state lives in a Zustand store persisted to `localStorage`. `react-resizable-panels` handles split-pane mechanics. Zero backend changes.

**Tech Stack:** React 19, TypeScript, Zustand, react-resizable-panels, xterm.js, TanStack Query, Tailwind CSS, vitest

**Spec:** `docs/superpowers/specs/2026-03-16-multi-view-terminal-design.md`

---

## File Map

### New Files

| File | Responsibility |
|------|---------------|
| `web/src/types/multiview.ts` | TypeScript interfaces: Workspace, Pane, LayoutConfig, LayoutPreset |
| `web/src/stores/multiview.ts` | Zustand store: workspace CRUD, focus management, localStorage persistence |
| `web/src/__tests__/components/multiview/SessionPicker.test.tsx` | Integration tests for session picker filtering and selection |
| `web/src/components/multiview/MultiviewPage.tsx` | Top-level page component: toolbar + layout rendering |
| `web/src/components/multiview/MultiviewToolbar.tsx` | Layout picker, workspace name, save/switcher controls |
| `web/src/components/multiview/LayoutPresetIcon.tsx` | Small SVG thumbnails for each layout preset |
| `web/src/components/multiview/PanelLayout.tsx` | Maps LayoutPreset to nested PanelGroup/Panel tree |
| `web/src/components/multiview/TerminalPane.tsx` | Single pane: header + terminal + focus border |
| `web/src/components/multiview/PaneHeader.tsx` | 24px header bar: session type icon, machine, dir, maximize button |
| `web/src/components/multiview/SessionPicker.tsx` | Modal with search, machine filter, session list for picking sessions |
| `web/src/components/multiview/EmptyMultiview.tsx` | Empty state when no workspace is loaded |
| `web/src/components/multiview/PaneEmptyState.tsx` | Empty state for a single pane (no session or stale session) |
| `web/src/__tests__/stores/multiview.test.ts` | Unit tests for the Zustand store |
| `web/src/__tests__/components/multiview/PanelLayout.test.tsx` | Unit tests for layout preset mapping |
| `web/src/__tests__/components/multiview/PaneHeader.test.tsx` | Unit tests for pane header |
| `web/src/__tests__/components/multiview/TerminalPane.test.tsx` | Unit tests for terminal pane focus |
| `web/src/__tests__/components/multiview/MultiviewPage.test.tsx` | Integration tests for full page |

### Modified Files

| File | Change |
|------|--------|
| `web/package.json` | Add `react-resizable-panels` dependency |
| `web/src/App.tsx` | Add `/multiview` and `/multiview/:workspaceId` routes |
| `web/src/components/layout/Sidebar.tsx` | Add "Multi-View" nav item |
| `web/src/hooks/useTerminalSession.ts` | Add optional `useWebGL` parameter |
| `web/src/components/terminal/TerminalView.tsx` | Forward `useWebGL` prop to `useTerminalSession` |
| `web/src/components/sessions/SessionCard.tsx` | Add optional checkbox for multi-select mode |
| `web/src/components/sessions/SessionList.tsx` | Pass through multi-select props |
| `web/src/views/SessionsPage.tsx` | Add multi-select mode + "Open in Multi-View" button |

---

## Chunk 1: Foundation — Types, Store, and Hook Modification

### Task 1: Install react-resizable-panels

**Files:**
- Modify: `web/package.json`

- [ ] **Step 1: Install the dependency**

Run:
```bash
cd web && npm install react-resizable-panels
```

- [ ] **Step 2: Verify installation**

Run:
```bash
cd web && node -e "require('react-resizable-panels'); console.log('OK')"
```
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "chore: add react-resizable-panels dependency"
```

---

### Task 2: Define multiview types

**Files:**
- Create: `web/src/types/multiview.ts`

- [ ] **Step 1: Create the types file**

```typescript
// Note: '5-grid' is added beyond the original spec to support the 5-pane layout
// transition described in the spec's "Layout Transitions" section (V[H[P,P,P], H[P,P]])
export type LayoutPreset =
  | '2-horizontal'
  | '2-vertical'
  | '3-columns'
  | '3-main-side'
  | '4-grid'
  | '5-grid'
  | '6-grid'
  | 'custom';

export interface LayoutConfig {
  readonly preset: LayoutPreset;
  readonly autoSaveId?: string;
}

export interface Pane {
  readonly id: string;
  readonly sessionId: string;
}

export interface Workspace {
  readonly id: string;
  readonly name: string | null;
  readonly layout: LayoutConfig;
  readonly panes: readonly Pane[];
  readonly createdAt: string;
  readonly updatedAt: string;
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/types/multiview.ts
git commit -m "feat: add multiview type definitions"
```

---

### Task 3: Write failing tests for multiview store

**Files:**
- Create: `web/src/__tests__/stores/multiview.test.ts`

- [ ] **Step 1: Write the test file**

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value; },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { store = {}; },
  };
})();
Object.defineProperty(globalThis, 'localStorage', { value: localStorageMock });

describe('multiview store', () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
  });

  it('should export useMultiviewStore', async () => {
    const mod = await import('../../stores/multiview');
    expect(mod.useMultiviewStore).toBeDefined();
  });

  it('should start with empty workspaces and no active workspace', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const state = useMultiviewStore.getState();
    expect(state.workspaces).toEqual([]);
    expect(state.activeWorkspace).toBeNull();
    expect(state.focusedPaneId).toBeNull();
  });

  it('should create a scratch workspace from session IDs', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['session-1', 'session-2']);

    const state = useMultiviewStore.getState();
    expect(state.activeWorkspace).not.toBeNull();
    expect(state.activeWorkspace!.name).toBeNull();
    expect(state.activeWorkspace!.panes).toHaveLength(2);
    expect(state.activeWorkspace!.panes[0].sessionId).toBe('session-1');
    expect(state.activeWorkspace!.panes[1].sessionId).toBe('session-2');
    expect(state.activeWorkspace!.layout.preset).toBe('2-horizontal');
  });

  it('should pick correct default layout for pane count', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2', 's3']);
    expect(useMultiviewStore.getState().activeWorkspace!.layout.preset).toBe('3-columns');

    createScratchWorkspace(['s1', 's2', 's3', 's4']);
    expect(useMultiviewStore.getState().activeWorkspace!.layout.preset).toBe('4-grid');

    createScratchWorkspace(['s1', 's2', 's3', 's4', 's5']);
    expect(useMultiviewStore.getState().activeWorkspace!.layout.preset).toBe('5-grid');

    createScratchWorkspace(['s1', 's2', 's3', 's4', 's5', 's6']);
    expect(useMultiviewStore.getState().activeWorkspace!.layout.preset).toBe('6-grid');
  });

  it('should save a workspace with a name', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, saveWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    saveWorkspace('My Workspace');

    const state = useMultiviewStore.getState();
    expect(state.workspaces).toHaveLength(1);
    expect(state.workspaces[0].name).toBe('My Workspace');
    expect(state.activeWorkspace!.name).toBe('My Workspace');
  });

  it('should delete a workspace', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, saveWorkspace, deleteWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    saveWorkspace('To Delete');

    const wsId = useMultiviewStore.getState().workspaces[0].id;
    deleteWorkspace(wsId);

    expect(useMultiviewStore.getState().workspaces).toHaveLength(0);
  });

  it('should set focused pane', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { setFocusedPane } = useMultiviewStore.getState();

    setFocusedPane('pane-1');
    expect(useMultiviewStore.getState().focusedPaneId).toBe('pane-1');

    setFocusedPane(null);
    expect(useMultiviewStore.getState().focusedPaneId).toBeNull();
  });

  it('should change layout preset', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, setLayoutPreset } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    setLayoutPreset('2-vertical');

    expect(useMultiviewStore.getState().activeWorkspace!.layout.preset).toBe('2-vertical');
  });

  it('should add a pane', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, addPane } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    addPane('s3');

    const state = useMultiviewStore.getState();
    expect(state.activeWorkspace!.panes).toHaveLength(3);
    expect(state.activeWorkspace!.layout.preset).toBe('3-columns');
  });

  it('should not add pane beyond 6', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, addPane } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2', 's3', 's4', 's5', 's6']);
    addPane('s7');

    expect(useMultiviewStore.getState().activeWorkspace!.panes).toHaveLength(6);
  });

  it('should remove a pane', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, removePane } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2', 's3']);
    const paneId = useMultiviewStore.getState().activeWorkspace!.panes[1].id;
    removePane(paneId);

    const state = useMultiviewStore.getState();
    expect(state.activeWorkspace!.panes).toHaveLength(2);
    expect(state.activeWorkspace!.layout.preset).toBe('2-horizontal');
  });

  it('should not remove pane below 2', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, removePane } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    const paneId = useMultiviewStore.getState().activeWorkspace!.panes[0].id;
    removePane(paneId);

    expect(useMultiviewStore.getState().activeWorkspace!.panes).toHaveLength(2);
  });

  it('should swap a session in a pane', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, swapSession } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    const paneId = useMultiviewStore.getState().activeWorkspace!.panes[0].id;
    swapSession(paneId, 'new-session');

    expect(useMultiviewStore.getState().activeWorkspace!.panes[0].sessionId).toBe('new-session');
  });

  it('should load a saved workspace', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, saveWorkspace, loadWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    saveWorkspace('Saved One');
    const wsId = useMultiviewStore.getState().workspaces[0].id;

    createScratchWorkspace(['s3', 's4', 's5']);
    loadWorkspace(wsId);

    const state = useMultiviewStore.getState();
    expect(state.activeWorkspace!.name).toBe('Saved One');
    expect(state.activeWorkspace!.panes).toHaveLength(2);
  });

  it('should rename a workspace', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, saveWorkspace, renameWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    saveWorkspace('Old Name');
    const wsId = useMultiviewStore.getState().workspaces[0].id;
    renameWorkspace(wsId, 'New Name');

    expect(useMultiviewStore.getState().workspaces[0].name).toBe('New Name');
    expect(useMultiviewStore.getState().activeWorkspace!.name).toBe('New Name');
  });

  it('should persist scratch workspace to localStorage', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);

    const stored = localStorage.getItem('claude-plane:multiview:scratch');
    expect(stored).not.toBeNull();
    expect(JSON.parse(stored!).panes).toHaveLength(2);
  });

  it('should persist saved workspaces to localStorage', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, saveWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    saveWorkspace('Persisted');

    const stored = localStorage.getItem('claude-plane:multiview:workspaces');
    expect(stored).not.toBeNull();
    expect(JSON.parse(stored!)).toHaveLength(1);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd web && npx vitest run src/__tests__/stores/multiview.test.ts
```
Expected: FAIL — module `../../stores/multiview` not found

- [ ] **Step 3: Commit failing tests**

```bash
git add web/src/__tests__/stores/multiview.test.ts
git commit -m "test: add failing tests for multiview store"
```

---

### Task 4: Implement multiview store

**Files:**
- Create: `web/src/stores/multiview.ts`

- [ ] **Step 1: Implement the store**

```typescript
import { create } from 'zustand';
import type { Workspace, Pane, LayoutPreset, LayoutConfig } from '../types/multiview';

const SCRATCH_KEY = 'claude-plane:multiview:scratch';
const WORKSPACES_KEY = 'claude-plane:multiview:workspaces';

function defaultPresetForCount(count: number): LayoutPreset {
  switch (count) {
    case 2: return '2-horizontal';
    case 3: return '3-columns';
    case 4: return '4-grid';
    case 5: return '5-grid';
    case 6: return '6-grid';
    default: return '2-horizontal';
  }
}

function makePanes(sessionIds: readonly string[]): readonly Pane[] {
  return sessionIds.map((sessionId) => ({
    id: crypto.randomUUID(),
    sessionId,
  }));
}

function makeWorkspace(sessionIds: readonly string[], name: string | null = null): Workspace {
  const now = new Date().toISOString();
  return {
    id: crypto.randomUUID(),
    name,
    layout: { preset: defaultPresetForCount(sessionIds.length) },
    panes: makePanes(sessionIds),
    createdAt: now,
    updatedAt: now,
  };
}

function persistScratch(workspace: Workspace | null): void {
  if (workspace) {
    localStorage.setItem(SCRATCH_KEY, JSON.stringify(workspace));
  } else {
    localStorage.removeItem(SCRATCH_KEY);
  }
}

function persistWorkspaces(workspaces: readonly Workspace[]): void {
  localStorage.setItem(WORKSPACES_KEY, JSON.stringify(workspaces));
}

function loadScratch(): Workspace | null {
  const raw = localStorage.getItem(SCRATCH_KEY);
  return raw ? (JSON.parse(raw) as Workspace) : null;
}

function loadWorkspaces(): Workspace[] {
  const raw = localStorage.getItem(WORKSPACES_KEY);
  return raw ? (JSON.parse(raw) as Workspace[]) : [];
}

function updateActiveWorkspace(
  workspace: Workspace,
  update: Partial<Pick<Workspace, 'name' | 'layout' | 'panes'>>,
): Workspace {
  return {
    ...workspace,
    ...update,
    updatedAt: new Date().toISOString(),
  };
}

interface MultiviewState {
  workspaces: readonly Workspace[];
  activeWorkspace: Workspace | null;
  focusedPaneId: string | null;
  createScratchWorkspace: (sessionIds: readonly string[]) => void;
  saveWorkspace: (name: string) => void;
  deleteWorkspace: (id: string) => void;
  loadWorkspace: (id: string) => void;
  renameWorkspace: (id: string, name: string) => void;
  setLayoutPreset: (preset: LayoutPreset) => void;
  setFocusedPane: (paneId: string | null) => void;
  addPane: (sessionId: string) => void;
  removePane: (paneId: string) => void;
  swapSession: (paneId: string, sessionId: string) => void;
}

export const useMultiviewStore = create<MultiviewState>((set, get) => ({
  workspaces: loadWorkspaces(),
  activeWorkspace: loadScratch(),
  focusedPaneId: null,

  createScratchWorkspace: (sessionIds) => {
    const workspace = makeWorkspace(sessionIds);
    persistScratch(workspace);
    set({ activeWorkspace: workspace, focusedPaneId: null });
  },

  saveWorkspace: (name) => {
    const { activeWorkspace, workspaces } = get();
    if (!activeWorkspace) return;

    const saved = { ...activeWorkspace, name, updatedAt: new Date().toISOString() };
    const existing = workspaces.findIndex((w) => w.id === saved.id);
    const updated = existing >= 0
      ? workspaces.map((w) => (w.id === saved.id ? saved : w))
      : [...workspaces, saved];

    persistWorkspaces(updated);
    persistScratch(saved);
    set({ workspaces: updated, activeWorkspace: saved });
  },

  deleteWorkspace: (id) => {
    const { workspaces, activeWorkspace } = get();
    const updated = workspaces.filter((w) => w.id !== id);
    persistWorkspaces(updated);

    if (activeWorkspace?.id === id) {
      persistScratch(null);
      set({ workspaces: updated, activeWorkspace: null, focusedPaneId: null });
    } else {
      set({ workspaces: updated });
    }
  },

  loadWorkspace: (id) => {
    const { workspaces } = get();
    const workspace = workspaces.find((w) => w.id === id) ?? null;
    if (workspace) {
      persistScratch(workspace);
      set({ activeWorkspace: workspace, focusedPaneId: null });
    }
  },

  renameWorkspace: (id, name) => {
    const { workspaces, activeWorkspace } = get();
    const updated = workspaces.map((w) =>
      w.id === id ? { ...w, name, updatedAt: new Date().toISOString() } : w,
    );
    persistWorkspaces(updated);

    const newActive =
      activeWorkspace?.id === id
        ? { ...activeWorkspace, name, updatedAt: new Date().toISOString() }
        : activeWorkspace;
    if (newActive !== activeWorkspace) persistScratch(newActive);
    set({ workspaces: updated, activeWorkspace: newActive });
  },

  setLayoutPreset: (preset) => {
    const { activeWorkspace } = get();
    if (!activeWorkspace) return;

    const updated = updateActiveWorkspace(activeWorkspace, {
      layout: { preset },
    });
    persistScratch(updated);
    set({ activeWorkspace: updated });
  },

  setFocusedPane: (paneId) => {
    set({ focusedPaneId: paneId });
  },

  addPane: (sessionId) => {
    const { activeWorkspace } = get();
    if (!activeWorkspace || activeWorkspace.panes.length >= 6) return;

    const newPanes = [...activeWorkspace.panes, { id: crypto.randomUUID(), sessionId }];
    const updated = updateActiveWorkspace(activeWorkspace, {
      panes: newPanes,
      layout: { preset: defaultPresetForCount(newPanes.length) },
    });
    persistScratch(updated);
    set({ activeWorkspace: updated });
  },

  removePane: (paneId) => {
    const { activeWorkspace, focusedPaneId } = get();
    if (!activeWorkspace || activeWorkspace.panes.length <= 2) return;

    const removedIndex = activeWorkspace.panes.findIndex((p) => p.id === paneId);
    const newPanes = activeWorkspace.panes.filter((p) => p.id !== paneId);
    const updated = updateActiveWorkspace(activeWorkspace, {
      panes: newPanes,
      layout: { preset: defaultPresetForCount(newPanes.length) },
    });

    const newFocus =
      focusedPaneId === paneId
        ? newPanes[Math.max(0, removedIndex - 1)]?.id ?? null
        : focusedPaneId;

    persistScratch(updated);
    set({ activeWorkspace: updated, focusedPaneId: newFocus });
  },

  swapSession: (paneId, sessionId) => {
    const { activeWorkspace } = get();
    if (!activeWorkspace) return;

    const newPanes = activeWorkspace.panes.map((p) =>
      p.id === paneId ? { ...p, sessionId } : p,
    );
    const updated = updateActiveWorkspace(activeWorkspace, { panes: newPanes });
    persistScratch(updated);
    set({ activeWorkspace: updated });
  },
}));
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```bash
cd web && npx vitest run src/__tests__/stores/multiview.test.ts
```
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/stores/multiview.ts
git commit -m "feat: implement multiview Zustand store with localStorage persistence"
```

---

### Task 5: Modify useTerminalSession to accept useWebGL parameter

**Files:**
- Modify: `web/src/hooks/useTerminalSession.ts:11-14` (function signature)
- Modify: `web/src/hooks/useTerminalSession.ts:45-50` (WebGL addon loading)

- [ ] **Step 1: Update function signature**

In `web/src/hooks/useTerminalSession.ts`, change the function signature:

```typescript
// Old (lines 11-14):
export function useTerminalSession(
  sessionId: string,
  containerRef: RefObject<HTMLDivElement | null>,
)

// New:
export function useTerminalSession(
  sessionId: string,
  containerRef: RefObject<HTMLDivElement | null>,
  options?: { useWebGL?: boolean },
)
```

- [ ] **Step 2: Conditionally load WebGL addon**

In `web/src/hooks/useTerminalSession.ts`, change the WebGL loading block:

```typescript
// Old (lines 45-50):
try {
  term.loadAddon(new WebglAddon());
} catch {
  // Falls back to canvas/DOM renderer silently
}

// New:
if (options?.useWebGL !== false) {
  try {
    term.loadAddon(new WebglAddon());
  } catch {
    // Falls back to canvas/DOM renderer silently
  }
}
```

- [ ] **Step 3: Update TerminalView to forward useWebGL**

In `web/src/components/terminal/TerminalView.tsx`, update the props interface (lines 5-9):

```typescript
interface TerminalViewProps {
  sessionId: string;
  onStatusChange?: (status: TerminalStatus) => void;
  className?: string;
  useWebGL?: boolean;
}
```

Update the component to destructure and forward the prop:

```typescript
export function TerminalView({ sessionId, onStatusChange, className = '', useWebGL }: TerminalViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { status } = useTerminalSession(sessionId, containerRef, { useWebGL });
```

- [ ] **Step 4: Verify existing tests still pass**

Run:
```bash
cd web && npx vitest run src/hooks/__tests__/useTerminalSession.test.ts
```
Expected: PASS (backward compatible — existing callers don't pass options)

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useTerminalSession.ts web/src/components/terminal/TerminalView.tsx
git commit -m "feat: add optional useWebGL parameter to useTerminalSession and TerminalView"
```

---

## Chunk 2: Layout Engine — PanelLayout and Preset Icons

### Task 6: Write failing tests for PanelLayout

**Files:**
- Create: `web/src/__tests__/components/multiview/PanelLayout.test.tsx`

- [ ] **Step 1: Write the test file**

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PanelLayout } from '../../../components/multiview/PanelLayout';
import type { LayoutPreset, Pane } from '../../../types/multiview';

// Mock react-resizable-panels
vi.mock('react-resizable-panels', () => ({
  PanelGroup: ({ children, ...props }: any) => (
    <div data-testid="panel-group" data-direction={props.direction}>{children}</div>
  ),
  Panel: ({ children, ...props }: any) => (
    <div data-testid="panel" data-min-size={props.minSize}>{children}</div>
  ),
  PanelResizeHandle: (props: any) => <div data-testid="resize-handle" />,
}));

const makePanes = (count: number): Pane[] =>
  Array.from({ length: count }, (_, i) => ({
    id: `pane-${i}`,
    sessionId: `session-${i}`,
  }));

const renderPane = (pane: { id: string; sessionId: string }) => (
  <div data-testid={`terminal-${pane.id}`}>{pane.sessionId}</div>
);

describe('PanelLayout', () => {
  it('renders 2-horizontal as two side-by-side panels', () => {
    render(
      <PanelLayout
        preset="2-horizontal"
        panes={makePanes(2)}
        renderPane={renderPane}
        workspaceId="ws-1"
      />,
    );
    const groups = screen.getAllByTestId('panel-group');
    expect(groups[0]).toHaveAttribute('data-direction', 'horizontal');
    expect(screen.getAllByTestId('panel')).toHaveLength(2);
  });

  it('renders 2-vertical as two stacked panels', () => {
    render(
      <PanelLayout
        preset="2-vertical"
        panes={makePanes(2)}
        renderPane={renderPane}
        workspaceId="ws-1"
      />,
    );
    const groups = screen.getAllByTestId('panel-group');
    expect(groups[0]).toHaveAttribute('data-direction', 'vertical');
  });

  it('renders 4-grid as 2x2 with nested groups', () => {
    render(
      <PanelLayout
        preset="4-grid"
        panes={makePanes(4)}
        renderPane={renderPane}
        workspaceId="ws-1"
      />,
    );
    const panels = screen.getAllByTestId('panel');
    expect(panels.length).toBeGreaterThanOrEqual(4);
    expect(screen.getByTestId('terminal-pane-0')).toBeDefined();
    expect(screen.getByTestId('terminal-pane-3')).toBeDefined();
  });

  it('renders all pane content via renderPane callback', () => {
    render(
      <PanelLayout
        preset="3-columns"
        panes={makePanes(3)}
        renderPane={renderPane}
        workspaceId="ws-1"
      />,
    );
    expect(screen.getByText('session-0')).toBeDefined();
    expect(screen.getByText('session-1')).toBeDefined();
    expect(screen.getByText('session-2')).toBeDefined();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd web && npx vitest run src/__tests__/components/multiview/PanelLayout.test.tsx
```
Expected: FAIL — module not found

- [ ] **Step 3: Commit**

```bash
git add web/src/__tests__/components/multiview/PanelLayout.test.tsx
git commit -m "test: add failing tests for PanelLayout component"
```

---

### Task 7: Implement PanelLayout

**Files:**
- Create: `web/src/components/multiview/PanelLayout.tsx`

- [ ] **Step 1: Create the PanelLayout component**

```tsx
import { Fragment, type ReactNode } from 'react';
import { PanelGroup, Panel, PanelResizeHandle } from 'react-resizable-panels';
import type { LayoutPreset, Pane } from '../../types/multiview';

interface PanelLayoutProps {
  readonly preset: LayoutPreset;
  readonly panes: readonly Pane[];
  readonly renderPane: (pane: Pane) => ReactNode;
  readonly workspaceId: string;
}

function ResizeHandle() {
  return (
    <PanelResizeHandle className="w-1 hover:w-1.5 bg-border-primary hover:bg-accent-primary transition-all duration-150" />
  );
}

function VerticalResizeHandle() {
  return (
    <PanelResizeHandle className="h-1 hover:h-1.5 bg-border-primary hover:bg-accent-primary transition-all duration-150" />
  );
}

function HorizontalRow({
  panes,
  renderPane,
  autoSaveId,
  minSize,
}: {
  readonly panes: readonly Pane[];
  readonly renderPane: (pane: Pane) => ReactNode;
  readonly autoSaveId: string;
  readonly minSize?: number;
}) {
  return (
    <PanelGroup direction="horizontal" autoSaveId={autoSaveId}>
      {panes.map((pane, i) => (
        <Fragment key={pane.id}>
          {i > 0 && <ResizeHandle />}
          <Panel minSize={minSize ?? 15}>
            {renderPane(pane)}
          </Panel>
        </Fragment>
      ))}
    </PanelGroup>
  );
}

export function PanelLayout({ preset, panes, renderPane, workspaceId }: PanelLayoutProps) {
  const baseId = `multiview-${workspaceId}`;

  switch (preset) {
    case '2-horizontal':
    case 'custom':
      return (
        <PanelGroup direction="horizontal" autoSaveId={`${baseId}-h`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <ResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </PanelGroup>
      );

    case '2-vertical':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <VerticalResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </PanelGroup>
      );

    case '3-columns':
      return (
        <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} autoSaveId={`${baseId}-h`} minSize={10} />
      );

    case '3-main-side':
      return (
        <PanelGroup direction="horizontal" autoSaveId={`${baseId}-h`}>
          <Panel defaultSize={66} minSize={30}>
            {renderPane(panes[0])}
          </Panel>
          <ResizeHandle />
          <Panel minSize={15}>
            <PanelGroup direction="vertical" autoSaveId={`${baseId}-v-right`}>
              <Panel minSize={20}>{renderPane(panes[1])}</Panel>
              <VerticalResizeHandle />
              <Panel minSize={20}>{renderPane(panes[2])}</Panel>
            </PanelGroup>
          </Panel>
        </PanelGroup>
      );

    case '4-grid':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 2)} renderPane={renderPane} autoSaveId={`${baseId}-h-top`} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(2, 4)} renderPane={renderPane} autoSaveId={`${baseId}-h-bot`} />
          </Panel>
        </PanelGroup>
      );

    case '5-grid':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} autoSaveId={`${baseId}-h-top`} minSize={10} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(3, 5)} renderPane={renderPane} autoSaveId={`${baseId}-h-bot`} />
          </Panel>
        </PanelGroup>
      );

    case '6-grid':
      return (
        <PanelGroup direction="vertical" autoSaveId={`${baseId}-v`}>
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(0, 3)} renderPane={renderPane} autoSaveId={`${baseId}-h-top`} minSize={10} />
          </Panel>
          <VerticalResizeHandle />
          <Panel minSize={20}>
            <HorizontalRow panes={panes.slice(3, 6)} renderPane={renderPane} autoSaveId={`${baseId}-h-bot`} minSize={10} />
          </Panel>
        </PanelGroup>
      );

    default:
      return (
        <PanelGroup direction="horizontal" autoSaveId={`${baseId}-h`}>
          <Panel minSize={15}>{renderPane(panes[0])}</Panel>
          <ResizeHandle />
          <Panel minSize={15}>{renderPane(panes[1])}</Panel>
        </PanelGroup>
      );
  }
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```bash
cd web && npx vitest run src/__tests__/components/multiview/PanelLayout.test.tsx
```
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/multiview/PanelLayout.tsx
git commit -m "feat: implement PanelLayout with all grid presets"
```

---

### Task 8: Implement LayoutPresetIcon

**Files:**
- Create: `web/src/components/multiview/LayoutPresetIcon.tsx`

- [ ] **Step 1: Create the preset icon component**

These are tiny inline SVGs that show a visual thumbnail of each layout:

```tsx
import type { LayoutPreset } from '../../types/multiview';

interface LayoutPresetIconProps {
  readonly preset: LayoutPreset;
  readonly size?: number;
  readonly active?: boolean;
}

function Box({ x, y, w, h }: { x: number; y: number; w: number; h: number }) {
  return <rect x={x} y={y} width={w} height={h} rx={1} fill="currentColor" opacity={0.3} stroke="currentColor" strokeWidth={0.5} />;
}

const layouts: Record<LayoutPreset, (s: number) => JSX.Element> = {
  '2-horizontal': (s) => (
    <>
      <Box x={1} y={1} w={s / 2 - 2} h={s - 2} />
      <Box x={s / 2 + 1} y={1} w={s / 2 - 2} h={s - 2} />
    </>
  ),
  '2-vertical': (s) => (
    <>
      <Box x={1} y={1} w={s - 2} h={s / 2 - 2} />
      <Box x={1} y={s / 2 + 1} w={s - 2} h={s / 2 - 2} />
    </>
  ),
  '3-columns': (s) => {
    const w = (s - 4) / 3;
    return (
      <>
        <Box x={1} y={1} w={w} h={s - 2} />
        <Box x={w + 2} y={1} w={w} h={s - 2} />
        <Box x={2 * w + 3} y={1} w={w} h={s - 2} />
      </>
    );
  },
  '3-main-side': (s) => {
    const mainW = (s - 3) * 0.66;
    const sideW = s - mainW - 3;
    const halfH = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={mainW} h={s - 2} />
        <Box x={mainW + 2} y={1} w={sideW} h={halfH} />
        <Box x={mainW + 2} y={halfH + 2} w={sideW} h={halfH} />
      </>
    );
  },
  '4-grid': (s) => {
    const half = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={half} h={half} />
        <Box x={half + 2} y={1} w={half} h={half} />
        <Box x={1} y={half + 2} w={half} h={half} />
        <Box x={half + 2} y={half + 2} w={half} h={half} />
      </>
    );
  },
  '5-grid': (s) => {
    const w3 = (s - 4) / 3;
    const w2 = (s - 3) / 2;
    const halfH = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={w3} h={halfH} />
        <Box x={w3 + 2} y={1} w={w3} h={halfH} />
        <Box x={2 * w3 + 3} y={1} w={w3} h={halfH} />
        <Box x={1} y={halfH + 2} w={w2} h={halfH} />
        <Box x={w2 + 2} y={halfH + 2} w={w2} h={halfH} />
      </>
    );
  },
  '6-grid': (s) => {
    const w = (s - 4) / 3;
    const halfH = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={w} h={halfH} />
        <Box x={w + 2} y={1} w={w} h={halfH} />
        <Box x={2 * w + 3} y={1} w={w} h={halfH} />
        <Box x={1} y={halfH + 2} w={w} h={halfH} />
        <Box x={w + 2} y={halfH + 2} w={w} h={halfH} />
        <Box x={2 * w + 3} y={halfH + 2} w={w} h={halfH} />
      </>
    );
  },
  custom: (s) => {
    const w1 = (s - 3) * 0.6;
    const w2 = s - w1 - 3;
    return (
      <>
        <Box x={1} y={1} w={w1} h={s - 2} />
        <Box x={w1 + 2} y={1} w={w2} h={s - 2} />
      </>
    );
  },
};

export function LayoutPresetIcon({ preset, size = 24, active = false }: LayoutPresetIconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className={active ? 'text-accent-primary' : 'text-text-secondary hover:text-text-primary'}
    >
      {layouts[preset](size)}
    </svg>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/multiview/LayoutPresetIcon.tsx
git commit -m "feat: add LayoutPresetIcon SVG thumbnails for grid presets"
```

---

## Chunk 3: Terminal Pane — Header, Focus, and Maximize

### Task 9: Write failing tests for PaneHeader

**Files:**
- Create: `web/src/__tests__/components/multiview/PaneHeader.test.tsx`

- [ ] **Step 1: Write the test file**

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PaneHeader } from '../../../components/multiview/PaneHeader';

describe('PaneHeader', () => {
  const defaultProps = {
    sessionType: 'claude' as const,
    machineName: 'worker-1',
    workingDir: '/home/user/projects/my-app',
    isMaximized: false,
    onMaximize: () => {},
  };

  it('renders machine name and working directory', () => {
    render(<PaneHeader {...defaultProps} />);
    expect(screen.getByText('worker-1')).toBeDefined();
    expect(screen.getByText(/my-app/)).toBeDefined();
  });

  it('truncates long working directory from the left', () => {
    render(
      <PaneHeader
        {...defaultProps}
        workingDir="/very/long/path/that/should/be/truncated/to/show/end"
      />,
    );
    // The component should show the end of the path
    expect(screen.getByText(/show\/end/)).toBeDefined();
  });

  it('shows maximize icon when not maximized', () => {
    render(<PaneHeader {...defaultProps} />);
    expect(screen.getByLabelText('Maximize pane')).toBeDefined();
  });

  it('shows minimize icon when maximized', () => {
    render(<PaneHeader {...defaultProps} isMaximized={true} />);
    expect(screen.getByLabelText('Restore pane')).toBeDefined();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd web && npx vitest run src/__tests__/components/multiview/PaneHeader.test.tsx
```
Expected: FAIL — module not found

- [ ] **Step 3: Commit**

```bash
git add web/src/__tests__/components/multiview/PaneHeader.test.tsx
git commit -m "test: add failing tests for PaneHeader component"
```

---

### Task 10: Implement PaneHeader

**Files:**
- Create: `web/src/components/multiview/PaneHeader.tsx`

- [ ] **Step 1: Create the PaneHeader component**

```tsx
import { Maximize2, Minimize2, Sparkles, TerminalSquare } from 'lucide-react';

interface PaneHeaderProps {
  readonly sessionType: 'claude' | 'terminal';
  readonly machineName: string;
  readonly workingDir: string;
  readonly isMaximized: boolean;
  readonly onMaximize: () => void;
}

function truncateDir(dir: string, maxLen: number = 30): string {
  if (dir.length <= maxLen) return dir;
  return '\u2026' + dir.slice(-(maxLen - 1));
}

export function PaneHeader({
  sessionType,
  machineName,
  workingDir,
  isMaximized,
  onMaximize,
}: PaneHeaderProps) {
  const Icon = sessionType === 'claude' ? Sparkles : TerminalSquare;

  return (
    <div className="flex items-center h-6 px-2 bg-bg-secondary border-b border-border-primary text-xs text-text-secondary select-none shrink-0">
      <Icon size={12} className="shrink-0 mr-1.5" />
      <span className="font-medium text-text-primary mr-2 shrink-0">{machineName}</span>
      <span className="truncate" title={workingDir}>
        {truncateDir(workingDir)}
      </span>
      <div className="ml-auto shrink-0">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onMaximize();
          }}
          className="p-0.5 rounded hover:bg-bg-tertiary transition-colors"
          aria-label={isMaximized ? 'Restore pane' : 'Maximize pane'}
        >
          {isMaximized ? <Minimize2 size={12} /> : <Maximize2 size={12} />}
        </button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```bash
cd web && npx vitest run src/__tests__/components/multiview/PaneHeader.test.tsx
```
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/multiview/PaneHeader.tsx
git commit -m "feat: implement PaneHeader with session info and maximize button"
```

---

### Task 11: Implement TerminalPane

**Files:**
- Create: `web/src/components/multiview/TerminalPane.tsx`
- Create: `web/src/components/multiview/PaneEmptyState.tsx`

- [ ] **Step 1: Create the PaneEmptyState component**

```tsx
interface PaneEmptyStateProps {
  readonly message?: string;
  readonly onPickSession: () => void;
}

export function PaneEmptyState({ message, onPickSession }: PaneEmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center h-full bg-bg-primary text-text-secondary">
      <p className="text-sm mb-3">{message ?? 'No session selected'}</p>
      <button
        onClick={onPickSession}
        className="px-3 py-1.5 text-sm rounded bg-accent-primary text-white hover:bg-accent-primary/80 transition-colors"
      >
        Pick a session
      </button>
    </div>
  );
}
```

- [ ] **Step 2: Create the TerminalPane component**

```tsx
import { useCallback, useMemo, useRef } from 'react';
import { TerminalView } from '../terminal/TerminalView';
import { PaneHeader } from './PaneHeader';
import { PaneEmptyState } from './PaneEmptyState';
import { useSession } from '../../hooks/useSessions';
import { useMachines } from '../../hooks/useMachines';
import type { Pane } from '../../types/multiview';

interface TerminalPaneProps {
  readonly pane: Pane;
  readonly isFocused: boolean;
  readonly isMaximized: boolean;
  readonly useWebGL: boolean;
  readonly onFocus: () => void;
  readonly onMaximize: () => void;
  readonly onPickSession: () => void;
}

function detectSessionType(command: string): 'claude' | 'terminal' {
  return command.toLowerCase().includes('claude') ? 'claude' : 'terminal';
}

export function TerminalPane({
  pane,
  isFocused,
  isMaximized,
  useWebGL,
  onFocus,
  onMaximize,
  onPickSession,
}: TerminalPaneProps) {
  const hasSession = pane.sessionId !== '';
  const { data: session } = useSession(hasSession ? pane.sessionId : '');
  const { data: machines } = useMachines();

  const machineName = useMemo(() => {
    if (!session || !machines) return '...';
    const machine = machines.find((m) => m.machine_id === session.machine_id);
    return machine?.display_name ?? machine?.machine_id?.slice(0, 8) ?? 'unknown';
  }, [session, machines]);

  const isStale = session && ['completed', 'failed', 'terminated'].includes(session.status);

  const borderClass = isFocused
    ? 'ring-2 ring-accent-primary shadow-[0_0_8px_rgba(99,102,241,0.3)]'
    : 'ring-1 ring-border-primary';

  return (
    <div
      className={`flex flex-col h-full rounded overflow-hidden ${borderClass} transition-shadow`}
      onClick={onFocus}
    >
      {session && (
        <PaneHeader
          sessionType={detectSessionType(session.command)}
          machineName={machineName}
          workingDir={session.working_dir}
          isMaximized={isMaximized}
          onMaximize={onMaximize}
        />
      )}
      <div className="flex-1 min-h-0 relative">
        {!hasSession ? (
          <PaneEmptyState onPickSession={onPickSession} />
        ) : !session ? (
          <PaneEmptyState
            message="Session no longer available"
            onPickSession={onPickSession}
          />
        ) : isStale ? (
          <div className="relative h-full">
            <TerminalView
              sessionId={pane.sessionId}
              className="h-full opacity-50"
              useWebGL={useWebGL}
            />
            <div className="absolute inset-0 flex items-center justify-center bg-black/40">
              <div className="text-center">
                <p className="text-text-secondary text-sm mb-2">Session ended</p>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onPickSession();
                  }}
                  className="px-3 py-1.5 text-xs rounded bg-bg-secondary text-text-primary hover:bg-bg-tertiary transition-colors"
                >
                  Swap session
                </button>
              </div>
            </div>
          </div>
        ) : (
          <TerminalView
            sessionId={pane.sessionId}
            className="h-full"
            useWebGL={useWebGL}
          />
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/multiview/TerminalPane.tsx web/src/components/multiview/PaneEmptyState.tsx
git commit -m "feat: implement TerminalPane with focus, stale session overlay, and empty state"
```

---

## Chunk 4: Session Picker and Workspace Toolbar

### Task 12: Implement SessionPicker

**Files:**
- Create: `web/src/components/multiview/SessionPicker.tsx`

- [ ] **Step 1: Create the SessionPicker component**

```tsx
import { useState, useMemo } from 'react';
import { X, Search, Sparkles, TerminalSquare } from 'lucide-react';
import { useSessions } from '../../hooks/useSessions';
import { useMachines } from '../../hooks/useMachines';
import { StatusBadge } from '../shared/StatusBadge';
import type { Session } from '../../types/session';

interface SessionPickerProps {
  readonly onSelect: (sessionId: string) => void;
  readonly onClose: () => void;
  readonly excludeSessionIds?: readonly string[];
}

export function SessionPicker({ onSelect, onClose, excludeSessionIds = [] }: SessionPickerProps) {
  const [search, setSearch] = useState('');
  const [machineFilter, setMachineFilter] = useState('all');

  const { data: sessions } = useSessions({ status: 'running' });
  const { data: machines } = useMachines();

  const machineMap = useMemo(() => {
    const map = new Map<string, string>();
    machines?.forEach((m) => map.set(m.machine_id, m.display_name ?? m.machine_id.slice(0, 8)));
    return map;
  }, [machines]);

  const filtered = useMemo(() => {
    if (!sessions) return [];
    return sessions.filter((s) => {
      if (machineFilter !== 'all' && s.machine_id !== machineFilter) return false;
      if (search) {
        const term = search.toLowerCase();
        const machineName = machineMap.get(s.machine_id) ?? '';
        return (
          s.session_id.toLowerCase().includes(term) ||
          s.command.toLowerCase().includes(term) ||
          s.working_dir.toLowerCase().includes(term) ||
          machineName.toLowerCase().includes(term)
        );
      }
      return true;
    });
  }, [sessions, machineFilter, search, machineMap]);

  const isExcluded = (id: string) => excludeSessionIds.includes(id);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-bg-secondary rounded-lg shadow-xl w-full max-w-lg max-h-[70vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-border-primary">
          <h3 className="text-sm font-semibold text-text-primary">Select Session</h3>
          <button onClick={onClose} className="p-1 rounded hover:bg-bg-tertiary">
            <X size={16} className="text-text-secondary" />
          </button>
        </div>

        {/* Search + Filter */}
        <div className="p-3 border-b border-border-primary space-y-2">
          <div className="relative">
            <Search size={14} className="absolute left-2.5 top-2.5 text-text-secondary" />
            <input
              type="text"
              placeholder="Search sessions..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full pl-8 pr-3 py-2 text-sm bg-bg-primary border border-border-primary rounded text-text-primary placeholder:text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent-primary"
              autoFocus
            />
          </div>
          <select
            value={machineFilter}
            onChange={(e) => setMachineFilter(e.target.value)}
            className="w-full px-3 py-1.5 text-sm bg-bg-primary border border-border-primary rounded text-text-primary"
          >
            <option value="all">All machines</option>
            {machines?.map((m) => (
              <option key={m.machine_id} value={m.machine_id}>
                {m.display_name ?? m.machine_id.slice(0, 8)}
              </option>
            ))}
          </select>
        </div>

        {/* Session List */}
        <div className="flex-1 overflow-y-auto p-2">
          {filtered.length === 0 && (
            <p className="text-center text-text-secondary text-sm py-8">No running sessions found</p>
          )}
          {filtered.map((session) => {
            const excluded = isExcluded(session.session_id);
            const isClaudeSession = session.command.toLowerCase().includes('claude');
            return (
              <button
                key={session.session_id}
                onClick={() => !excluded && onSelect(session.session_id)}
                disabled={excluded}
                className={`w-full text-left px-3 py-2 rounded mb-1 flex items-center gap-2 text-sm transition-colors ${
                  excluded
                    ? 'opacity-40 cursor-not-allowed'
                    : 'hover:bg-bg-tertiary cursor-pointer'
                }`}
              >
                {isClaudeSession ? (
                  <Sparkles size={14} className="text-accent-primary shrink-0" />
                ) : (
                  <TerminalSquare size={14} className="text-text-secondary shrink-0" />
                )}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs text-text-secondary">
                      {session.session_id.slice(0, 8)}
                    </span>
                    <StatusBadge status={session.status} />
                    {excluded && (
                      <span className="text-xs text-text-secondary">Already in view</span>
                    )}
                  </div>
                  <div className="text-xs text-text-secondary truncate mt-0.5">
                    {machineMap.get(session.machine_id) ?? session.machine_id.slice(0, 8)} &middot; {session.working_dir}
                  </div>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/multiview/SessionPicker.tsx
git commit -m "feat: implement SessionPicker modal with search and machine filter"
```

---

### Task 13: Implement MultiviewToolbar

**Files:**
- Create: `web/src/components/multiview/MultiviewToolbar.tsx`

- [ ] **Step 1: Create the toolbar component**

```tsx
import { useState, useRef, useEffect } from 'react';
import { Save, Copy, ChevronDown, Plus, Trash2 } from 'lucide-react';
import { ConfirmDialog } from '../shared/ConfirmDialog';
import { LayoutPresetIcon } from './LayoutPresetIcon';
import { useMultiviewStore } from '../../stores/multiview';
import type { LayoutPreset, Workspace } from '../../types/multiview';

const PRESETS: readonly LayoutPreset[] = [
  '2-horizontal', '2-vertical',
  '3-columns', '3-main-side',
  '4-grid', '5-grid', '6-grid',
];

const PRESET_LABELS: Record<LayoutPreset, string> = {
  '2-horizontal': '2 Side by Side',
  '2-vertical': '2 Stacked',
  '3-columns': '3 Columns',
  '3-main-side': '1 Main + 2 Side',
  '4-grid': '2×2 Grid',
  '5-grid': '3+2 Grid',
  '6-grid': '2×3 Grid',
  custom: 'Custom',
};

export function MultiviewToolbar() {
  const {
    activeWorkspace,
    workspaces,
    setLayoutPreset,
    saveWorkspace,
    loadWorkspace,
    deleteWorkspace,
    renameWorkspace,
    addPane,
  } = useMultiviewStore();

  const [isEditing, setIsEditing] = useState(false);
  const [editName, setEditName] = useState('');
  const [showSwitcher, setShowSwitcher] = useState(false);
  const [showSavePrompt, setShowSavePrompt] = useState(false);
  const [saveName, setSaveName] = useState('');
  const [workspaceToDelete, setWorkspaceToDelete] = useState<Workspace | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const switcherRef = useRef<HTMLDivElement>(null);

  const currentPreset = activeWorkspace?.layout.preset ?? '2-horizontal';
  const paneCount = activeWorkspace?.panes.length ?? 0;

  // Available presets based on current pane count
  const availablePresets = PRESETS.filter((p) => {
    const minPanes: Record<LayoutPreset, number> = {
      '2-horizontal': 2, '2-vertical': 2,
      '3-columns': 3, '3-main-side': 3,
      '4-grid': 4, '5-grid': 5, '6-grid': 6,
      custom: 2,
    };
    return minPanes[p] <= paneCount;
  });

  useEffect(() => {
    if (isEditing && nameInputRef.current) {
      nameInputRef.current.focus();
      nameInputRef.current.select();
    }
  }, [isEditing]);

  // Close switcher on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (switcherRef.current && !switcherRef.current.contains(e.target as Node)) {
        setShowSwitcher(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const handleSave = () => {
    if (activeWorkspace?.name) {
      saveWorkspace(activeWorkspace.name);
    } else {
      setShowSavePrompt(true);
      setSaveName('');
    }
  };

  const handleSaveConfirm = () => {
    if (saveName.trim()) {
      saveWorkspace(saveName.trim());
      setShowSavePrompt(false);
    }
  };

  const handleRename = () => {
    if (activeWorkspace && editName.trim()) {
      renameWorkspace(activeWorkspace.id, editName.trim());
    }
    setIsEditing(false);
  };

  return (
    <div className="flex items-center gap-3 px-4 py-2 border-b border-border-primary bg-bg-secondary shrink-0">
      {/* Layout Presets */}
      <div className="flex items-center gap-1">
        {availablePresets.map((preset) => (
          <button
            key={preset}
            onClick={() => setLayoutPreset(preset)}
            className={`p-1 rounded transition-colors ${
              currentPreset === preset ? 'bg-bg-tertiary' : 'hover:bg-bg-tertiary'
            }`}
            title={PRESET_LABELS[preset]}
          >
            <LayoutPresetIcon preset={preset} size={20} active={currentPreset === preset} />
          </button>
        ))}
      </div>

      <div className="w-px h-5 bg-border-primary" />

      {/* Workspace Name */}
      <div className="flex items-center gap-2">
        {isEditing ? (
          <input
            ref={nameInputRef}
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            onBlur={handleRename}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleRename();
              if (e.key === 'Escape') setIsEditing(false);
            }}
            className="px-2 py-0.5 text-sm bg-bg-primary border border-border-primary rounded text-text-primary focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
        ) : (
          <button
            onClick={() => {
              setEditName(activeWorkspace?.name ?? '');
              setIsEditing(true);
            }}
            className="text-sm text-text-primary hover:text-accent-primary transition-colors"
          >
            {activeWorkspace?.name ?? 'Untitled workspace'}
          </button>
        )}

        {/* Save */}
        <button
          onClick={handleSave}
          className="p-1 rounded hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          title="Save workspace"
        >
          <Save size={14} />
        </button>

        {/* Save As */}
        <button
          onClick={() => { setShowSavePrompt(true); setSaveName(''); }}
          className="p-1 rounded hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          title="Save as new workspace"
        >
          <Copy size={14} />
        </button>

        {/* Workspace Switcher */}
        <div className="relative" ref={switcherRef}>
          <button
            onClick={() => setShowSwitcher(!showSwitcher)}
            className="p-1 rounded hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
            title="Switch workspace"
          >
            <ChevronDown size={14} />
          </button>
          {showSwitcher && (
            <div className="absolute top-full left-0 mt-1 w-56 bg-bg-secondary border border-border-primary rounded-lg shadow-xl z-50">
              {workspaces.map((ws) => (
                <div key={ws.id} className="flex items-center px-3 py-2 hover:bg-bg-tertiary group">
                  <button
                    onClick={() => {
                      loadWorkspace(ws.id);
                      setShowSwitcher(false);
                    }}
                    className="flex-1 text-left text-sm text-text-primary truncate"
                  >
                    {ws.name ?? 'Untitled'}
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setWorkspaceToDelete(ws);
                    }}
                    className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-bg-primary text-text-secondary"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              ))}
              {workspaces.length === 0 && (
                <p className="px-3 py-2 text-xs text-text-secondary">No saved workspaces</p>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex-1" />

      {/* Add Pane */}
      {paneCount < 6 && (
        <button
          onClick={() => addPane('')}
          className="flex items-center gap-1 px-2 py-1 text-xs rounded bg-bg-tertiary text-text-secondary hover:text-text-primary hover:bg-border-primary transition-colors"
        >
          <Plus size={12} />
          Add pane
        </button>
      )}

      {/* Delete Confirmation */}
      {workspaceToDelete && (
        <ConfirmDialog
          title="Delete Workspace"
          message={`Delete workspace "${workspaceToDelete.name}"? This cannot be undone.`}
          onConfirm={() => {
            deleteWorkspace(workspaceToDelete.id);
            setWorkspaceToDelete(null);
          }}
          onCancel={() => setWorkspaceToDelete(null)}
        />
      )}

      {/* Save Prompt Modal */}
      {showSavePrompt && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowSavePrompt(false)}>
          <div className="bg-bg-secondary p-4 rounded-lg shadow-xl w-80" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-text-primary mb-3">Save Workspace</h3>
            <input
              value={saveName}
              onChange={(e) => setSaveName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSaveConfirm()}
              placeholder="Workspace name"
              className="w-full px-3 py-2 text-sm bg-bg-primary border border-border-primary rounded text-text-primary mb-3 focus:outline-none focus:ring-1 focus:ring-accent-primary"
              autoFocus
            />
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowSavePrompt(false)}
                className="px-3 py-1.5 text-xs rounded text-text-secondary hover:text-text-primary"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveConfirm}
                disabled={!saveName.trim()}
                className="px-3 py-1.5 text-xs rounded bg-accent-primary text-white hover:bg-accent-primary/80 disabled:opacity-50"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/multiview/MultiviewToolbar.tsx
git commit -m "feat: implement MultiviewToolbar with layout picker, save, and workspace switcher"
```

---

## Chunk 5: Main Page, Routing, and Sidebar

### Task 14: Implement EmptyMultiview

**Files:**
- Create: `web/src/components/multiview/EmptyMultiview.tsx`

- [ ] **Step 1: Create the empty state component**

```tsx
import { LayoutGrid } from 'lucide-react';
import { useNavigate } from 'react-router';

interface EmptyMultiviewProps {
  readonly onCreateWorkspace?: () => void;
}

export function EmptyMultiview({ onCreateWorkspace }: EmptyMultiviewProps) {
  const navigate = useNavigate();

  return (
    <div className="flex flex-col items-center justify-center h-full text-center px-4">
      <LayoutGrid size={48} className="text-text-secondary mb-4" strokeWidth={1} />
      <h2 className="text-lg font-semibold text-text-primary mb-2">Multi-View</h2>
      <p className="text-sm text-text-secondary mb-6 max-w-md">
        View and interact with multiple terminal sessions simultaneously.
        Select sessions from the sessions page or create a new workspace.
      </p>
      <div className="flex gap-3">
        <button
          onClick={() => navigate('/sessions')}
          className="px-4 py-2 text-sm rounded bg-bg-secondary border border-border-primary text-text-primary hover:bg-bg-tertiary transition-colors"
        >
          Go to Sessions
        </button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/multiview/EmptyMultiview.tsx
git commit -m "feat: add EmptyMultiview placeholder for empty workspace state"
```

---

### Task 15: Implement MultiviewPage

**Files:**
- Create: `web/src/components/multiview/MultiviewPage.tsx`

- [ ] **Step 1: Create the main page component**

```tsx
import { useState, useCallback, useEffect } from 'react';
import { useParams } from 'react-router';
import { MultiviewToolbar } from './MultiviewToolbar';
import { PanelLayout } from './PanelLayout';
import { TerminalPane } from './TerminalPane';
import { SessionPicker } from './SessionPicker';
import { EmptyMultiview } from './EmptyMultiview';
import { useMultiviewStore } from '../../stores/multiview';
import type { Pane } from '../../types/multiview';

export function MultiviewPage() {
  const { workspaceId } = useParams<{ workspaceId?: string }>();
  const {
    activeWorkspace,
    focusedPaneId,
    loadWorkspace,
    setFocusedPane,
    swapSession,
    addPane,
  } = useMultiviewStore();

  const [maximizedPaneId, setMaximizedPaneId] = useState<string | null>(null);
  const [pickerTarget, setPickerTarget] = useState<string | null>(null); // paneId or '__new__'

  // Load workspace from URL param
  useEffect(() => {
    if (workspaceId) {
      loadWorkspace(workspaceId);
    }
  }, [workspaceId, loadWorkspace]);

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!activeWorkspace) return;
      const panes = activeWorkspace.panes;

      // Ctrl+Shift+M — toggle maximize
      if (e.ctrlKey && e.shiftKey && e.key === 'M') {
        e.preventDefault();
        if (focusedPaneId) {
          setMaximizedPaneId((prev) => (prev === focusedPaneId ? null : focusedPaneId));
        }
        return;
      }

      // Ctrl+Shift+1-6 — jump to pane by number
      // Use e.code (Digit1-Digit6) because e.key produces shifted chars (!, @, etc.) when Shift is held
      if (e.ctrlKey && e.shiftKey && e.code >= 'Digit1' && e.code <= 'Digit6') {
        e.preventDefault();
        const index = parseInt(e.code.replace('Digit', '')) - 1;
        if (index < panes.length) {
          setFocusedPane(panes[index].id);
        }
        return;
      }

      // Escape — unfocus
      if (e.key === 'Escape') {
        setFocusedPane(null);
        return;
      }

      // Ctrl+Shift+Arrow — directional focus (simplified: prev/next in reading order)
      if (e.ctrlKey && e.shiftKey && ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) {
        e.preventDefault();
        if (!focusedPaneId) return;
        const currentIndex = panes.findIndex((p) => p.id === focusedPaneId);
        if (currentIndex < 0) return;

        let nextIndex = currentIndex;
        if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
          nextIndex = Math.min(currentIndex + 1, panes.length - 1);
        } else {
          nextIndex = Math.max(currentIndex - 1, 0);
        }
        if (nextIndex !== currentIndex) {
          setFocusedPane(panes[nextIndex].id);
        }
      }
    };

    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [activeWorkspace, focusedPaneId, setFocusedPane]);

  const handlePickerSelect = useCallback(
    (sessionId: string) => {
      if (pickerTarget === '__new__') {
        addPane(sessionId);
      } else if (pickerTarget) {
        swapSession(pickerTarget, sessionId);
      }
      setPickerTarget(null);
    },
    [pickerTarget, addPane, swapSession],
  );

  const useWebGL = (activeWorkspace?.panes.length ?? 0) <= 4;

  if (!activeWorkspace) {
    return (
      <div className="flex flex-col h-full">
        <EmptyMultiview />
      </div>
    );
  }

  const excludeSessionIds = activeWorkspace.panes.map((p) => p.sessionId).filter(Boolean);

  const renderPane = (pane: Pane) => {
    if (maximizedPaneId && pane.id !== maximizedPaneId) return null;

    return (
      <TerminalPane
        key={pane.id}
        pane={pane}
        isFocused={focusedPaneId === pane.id}
        isMaximized={maximizedPaneId === pane.id}
        useWebGL={useWebGL}
        onFocus={() => setFocusedPane(pane.id)}
        onMaximize={() =>
          setMaximizedPaneId((prev) => (prev === pane.id ? null : pane.id))
        }
        onPickSession={() => setPickerTarget(pane.id)}
      />
    );
  };

  // When maximized, show only that pane
  if (maximizedPaneId) {
    const pane = activeWorkspace.panes.find((p) => p.id === maximizedPaneId);
    if (pane) {
      return (
        <div className="flex flex-col h-full">
          <MultiviewToolbar />
          <div className="flex-1 min-h-0 p-1">{renderPane(pane)}</div>
          {pickerTarget && (
            <SessionPicker
              onSelect={handlePickerSelect}
              onClose={() => setPickerTarget(null)}
              excludeSessionIds={excludeSessionIds}
            />
          )}
        </div>
      );
    }
  }

  return (
    <div className="flex flex-col h-full">
      <MultiviewToolbar />
      <div className="flex-1 min-h-0 p-1">
        <PanelLayout
          preset={activeWorkspace.layout.preset}
          panes={[...activeWorkspace.panes]}
          renderPane={renderPane}
          workspaceId={activeWorkspace.id}
        />
      </div>
      {pickerTarget && (
        <SessionPicker
          onSelect={handlePickerSelect}
          onClose={() => setPickerTarget(null)}
          excludeSessionIds={excludeSessionIds}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/multiview/MultiviewPage.tsx
git commit -m "feat: implement MultiviewPage with keyboard shortcuts, maximize, and session picker"
```

---

### Task 16: Add routes and sidebar entry

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx`

- [ ] **Step 1: Add import in App.tsx**

In `web/src/App.tsx`, add a direct import with the other view imports (the codebase does not use `lazy()` — all views are directly imported):

```typescript
import { MultiviewPage } from './components/multiview/MultiviewPage';
```

- [ ] **Step 2: Add routes in App.tsx**

In `web/src/App.tsx`, add after the `/sessions/:sessionId` route (around line 85):

```tsx
<Route path="/multiview" element={<MultiviewPage />} />
<Route path="/multiview/:workspaceId" element={<MultiviewPage />} />
```

- [ ] **Step 3: Add sidebar nav item**

In `web/src/components/layout/Sidebar.tsx`, add `LayoutGrid` to the lucide-react imports (lines 2-19). In the Core section's `items` array (lines 39-47), insert the Multi-View item at index 2 — between Sessions and Machines:

```typescript
// Core section items array should become:
{ to: '/', label: 'Command Center', icon: LayoutDashboard },
{ to: '/sessions', label: 'Sessions', icon: Terminal },
{ to: '/multiview', label: 'Multi-View', icon: LayoutGrid },  // <-- insert here
{ to: '/machines', label: 'Machines', icon: Server },
// ... rest of items
```

- [ ] **Step 4: Verify the app compiles**

Run:
```bash
cd web && npx tsc --noEmit
```
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add web/src/App.tsx web/src/components/layout/Sidebar.tsx
git commit -m "feat: add multiview routes and sidebar navigation entry"
```

---

## Chunk 6: Sessions Page Multi-Select Entry Point

### Task 17: Add multi-select to SessionCard

**Files:**
- Modify: `web/src/components/sessions/SessionCard.tsx:6-10` (props interface)

- [ ] **Step 1: Update SessionCard props and add checkbox**

In `web/src/components/sessions/SessionCard.tsx`, update the props interface:

```typescript
interface SessionCardProps {
  session: Session;
  onAttach: (id: string) => void;
  onTerminate: (id: string) => void;
  selectable?: boolean;
  selected?: boolean;
  onSelect?: (id: string) => void;
}
```

Add the checkbox rendering at the top of the card (inside the outer `<div>`, before the status badge area):

```tsx
{selectable && (
  <div className="absolute top-2 left-2 z-10">
    <input
      type="checkbox"
      checked={selected}
      onChange={() => onSelect?.(session.session_id)}
      className="w-4 h-4 rounded border-border-primary accent-accent-primary cursor-pointer"
      onClick={(e) => e.stopPropagation()}
    />
  </div>
)}
```

Make sure to add `relative` to the card's outer div className if not already present.

- [ ] **Step 2: Update SessionList to pass through props**

In `web/src/components/sessions/SessionList.tsx`, update the props interface and pass through:

```typescript
interface SessionListProps {
  sessions: Session[];
  onAttach: (id: string) => void;
  onTerminate: (id: string) => void;
  emptyMessage?: string;
  selectable?: boolean;
  selectedIds?: ReadonlySet<string>;
  onSelect?: (id: string) => void;
}
```

Pass the new props to each `SessionCard`:

```tsx
<SessionCard
  key={s.session_id}
  session={s}
  onAttach={onAttach}
  onTerminate={onTerminate}
  selectable={selectable}
  selected={selectedIds?.has(s.session_id)}
  onSelect={onSelect}
/>
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/sessions/SessionCard.tsx web/src/components/sessions/SessionList.tsx
git commit -m "feat: add multi-select checkbox support to SessionCard and SessionList"
```

---

### Task 18: Add "Open in Multi-View" to SessionsPage

**Files:**
- Modify: `web/src/views/SessionsPage.tsx`

- [ ] **Step 1: Add multi-select state and navigation**

In `web/src/views/SessionsPage.tsx`, add imports:

```typescript
import { LayoutGrid } from 'lucide-react';
import { useMultiviewStore } from '../stores/multiview';
```

Add state near the existing state declarations (around line 18):

```typescript
const [multiSelectMode, setMultiSelectMode] = useState(false);
const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
const createScratchWorkspace = useMultiviewStore((s) => s.createScratchWorkspace);
```

Add the selection handler:

```typescript
const handleSelect = useCallback((id: string) => {
  setSelectedIds((prev) => {
    const next = new Set(prev);
    if (next.has(id)) {
      next.delete(id);
    } else if (next.size < 6) {
      next.add(id);
    }
    return next;
  });
}, []);

const handleOpenMultiView = useCallback(() => {
  if (selectedIds.size >= 2) {
    createScratchWorkspace([...selectedIds]);
    navigate('/multiview');
  }
}, [selectedIds, createScratchWorkspace, navigate]);
```

- [ ] **Step 2: Add multi-select toggle button to the header**

Add a button next to the existing "New Session" button:

```tsx
<button
  onClick={() => {
    setMultiSelectMode(!multiSelectMode);
    setSelectedIds(new Set());
  }}
  className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded transition-colors ${
    multiSelectMode
      ? 'bg-accent-primary text-white'
      : 'bg-bg-secondary border border-border-primary text-text-primary hover:bg-bg-tertiary'
  }`}
>
  <LayoutGrid size={14} />
  Multi-View
</button>
```

- [ ] **Step 3: Pass multi-select props to SessionList**

Update the SessionList usage:

```tsx
<SessionList
  sessions={sessions}
  onAttach={handleAttach}
  onTerminate={confirmTerminate}
  emptyMessage="No sessions found"
  selectable={multiSelectMode}
  selectedIds={selectedIds}
  onSelect={handleSelect}
/>
```

- [ ] **Step 4: Add floating action bar when 2+ selected**

Add at the bottom of the page, before the closing `</div>`:

```tsx
{multiSelectMode && selectedIds.size >= 2 && (
  <div className="fixed bottom-6 left-1/2 -translate-x-1/2 bg-bg-secondary border border-border-primary rounded-lg shadow-xl px-4 py-2.5 flex items-center gap-3 z-40">
    <span className="text-sm text-text-secondary">{selectedIds.size} sessions selected</span>
    <button
      onClick={handleOpenMultiView}
      className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded bg-accent-primary text-white hover:bg-accent-primary/80 transition-colors"
    >
      <LayoutGrid size={14} />
      Open in Multi-View
    </button>
  </div>
)}
```

- [ ] **Step 5: Verify the app compiles**

Run:
```bash
cd web && npx tsc --noEmit
```
Expected: No type errors

- [ ] **Step 6: Commit**

```bash
git add web/src/views/SessionsPage.tsx
git commit -m "feat: add multi-select mode and Open in Multi-View to SessionsPage"
```

---

## Chunk 7: Context Menu, Resize Debouncing, and Final Integration

### Task 19: Add resize debouncing to useTerminalSession

**Files:**
- Modify: `web/src/hooks/useTerminalSession.ts:134-137` (ResizeObserver)

- [ ] **Step 1: Add debounced resize**

In `web/src/hooks/useTerminalSession.ts`, replace the ResizeObserver block:

```typescript
// Old (lines 134-137):
const observer = new ResizeObserver(() => {
  fitAddon.fit();
});

// New:
let resizeTimer: ReturnType<typeof setTimeout> | null = null;
const observer = new ResizeObserver(() => {
  if (resizeTimer) clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    fitAddon.fit();
  }, 50);
});
```

Make sure to clear the timer in the cleanup function (around line 140-156), add:

```typescript
if (resizeTimer) clearTimeout(resizeTimer);
```

- [ ] **Step 2: Verify existing tests still pass**

Run:
```bash
cd web && npx vitest run
```
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/useTerminalSession.ts
git commit -m "perf: debounce terminal resize to 50ms for multi-pane performance"
```

---

### Task 20: Add pane context menu to PaneHeader

**Files:**
- Modify: `web/src/components/multiview/PaneHeader.tsx`

- [ ] **Step 1: Add context menu props and handler**

Update the PaneHeader props interface:

```typescript
interface PaneHeaderProps {
  readonly sessionType: 'claude' | 'terminal';
  readonly machineName: string;
  readonly workingDir: string;
  readonly isMaximized: boolean;
  readonly onMaximize: () => void;
  readonly onSwapSession?: () => void;
  readonly onRemovePane?: () => void;
  readonly onOpenFullView?: () => void;
  readonly canRemove?: boolean;
}
```

Add context menu state and rendering inside the component:

```tsx
const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null);

const handleContextMenu = (e: React.MouseEvent) => {
  e.preventDefault();
  setContextMenu({ x: e.clientX, y: e.clientY });
};
```

Add `onContextMenu={handleContextMenu}` to the header's outer `<div>`.

Add the context menu dropdown:

```tsx
{contextMenu && (
  <>
    <div className="fixed inset-0 z-50" onClick={() => setContextMenu(null)} />
    <div
      className="fixed z-50 bg-bg-secondary border border-border-primary rounded-lg shadow-xl py-1 min-w-40"
      style={{ left: contextMenu.x, top: contextMenu.y }}
    >
      {onSwapSession && (
        <button
          onClick={() => { onSwapSession(); setContextMenu(null); }}
          className="w-full text-left px-3 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary"
        >
          Swap session...
        </button>
      )}
      {onRemovePane && canRemove && (
        <button
          onClick={() => { onRemovePane(); setContextMenu(null); }}
          className="w-full text-left px-3 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary"
        >
          Remove pane
        </button>
      )}
      {onOpenFullView && (
        <button
          onClick={() => { onOpenFullView(); setContextMenu(null); }}
          className="w-full text-left px-3 py-1.5 text-sm text-text-primary hover:bg-bg-tertiary"
        >
          Open in full view
        </button>
      )}
    </div>
  </>
)}
```

- [ ] **Step 2: Update TerminalPane to pass context menu props**

In `web/src/components/multiview/TerminalPane.tsx`, add the new props to the PaneHeader usage:

```tsx
<PaneHeader
  sessionType={detectSessionType(session.command)}
  machineName={machineName}
  workingDir={session.working_dir}
  isMaximized={isMaximized}
  onMaximize={onMaximize}
  onSwapSession={onPickSession}
  onRemovePane={onRemovePane}
  onOpenFullView={() => navigate(`/sessions/${pane.sessionId}`)}
  canRemove={canRemove}
/>
```

Add the new props to the TerminalPane interface and implementation:

```typescript
readonly onRemovePane?: () => void;
readonly canRemove?: boolean;
```

Add `useNavigate` import and usage.

- [ ] **Step 3: Update MultiviewPage to pass remove props**

In `web/src/components/multiview/MultiviewPage.tsx`, update the renderPane callback to pass:

```tsx
onRemovePane={() => removePane(pane.id)}
canRemove={activeWorkspace.panes.length > 2}
```

Add `removePane` to the destructured store methods.

- [ ] **Step 4: Verify the app compiles**

Run:
```bash
cd web && npx tsc --noEmit
```
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add web/src/components/multiview/PaneHeader.tsx web/src/components/multiview/TerminalPane.tsx web/src/components/multiview/MultiviewPage.tsx
git commit -m "feat: add pane context menu with swap, remove, and open full view"
```

---

### Task 21: Run full test suite and fix issues

- [ ] **Step 1: Run all frontend tests**

Run:
```bash
cd web && npx vitest run
```
Expected: All tests PASS

- [ ] **Step 2: Run typecheck**

Run:
```bash
cd web && npx tsc --noEmit
```
Expected: No type errors

- [ ] **Step 3: Run linter**

Run:
```bash
cd web && npm run lint
```
Expected: No lint errors (or only pre-existing ones)

- [ ] **Step 4: Build the frontend**

Run:
```bash
cd web && npm run build
```
Expected: Build succeeds

- [ ] **Step 5: Fix any issues found in steps 1-4**

Address each error individually, then re-run the failing check.

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "fix: resolve any remaining build/test/lint issues for multiview feature"
```

---

### Task 22: Run Go build to verify no regressions

- [ ] **Step 1: Build the frontend for embedding**

Run:
```bash
cd web && npm run build
```
Expected: Output to `internal/server/frontend/dist/`

- [ ] **Step 2: Build Go server binary**

Run:
```bash
go build -o claude-plane-server ./cmd/server
```
Expected: Build succeeds (frontend embedded via `go:embed`)

- [ ] **Step 3: Run Go tests**

Run:
```bash
go test -race ./...
```
Expected: All tests PASS (no backend changes, so this should be clean)

- [ ] **Step 4: Commit if any build adjustments needed**

```bash
git add -A
git commit -m "chore: verify full build chain with multiview feature"
```
