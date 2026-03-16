# Multi-View Terminal Design Spec

## Overview

A new multi-view page that displays 2-6 terminal sessions simultaneously in a configurable grid layout, enabling users to monitor and interact with multiple Claude CLI and shell sessions from a single screen.

## Goals

- Let users see and interact with multiple sessions without switching between pages
- Support both monitoring (watching sessions progress) and active multi-tasking (typing into multiple sessions)
- Provide professional, polished split-pane UX comparable to tmux or VS Code terminal panels
- Enable saved workspace configurations for recurring multi-repo setups

## Non-Goals

- Backend changes (no new APIs, no WebSocket multiplexing)
- Mobile/responsive support (multi-view is a desktop power-user feature)
- Session creation from within multi-view (users create sessions elsewhere, then add them to the view)

## Architecture

**Approach:** Single page with shared event WebSocket. Each terminal pane gets its own WebSocket connection (`/ws/terminal/{sessionId}`), sharing the existing global event WebSocket (`/ws/events`). Layout state lives entirely in the frontend via a Zustand store persisted to `localStorage`. Zero backend changes required.

**Key library:** `react-resizable-panels` (~3KB gzipped, zero dependencies) for split-pane layout management with drag-to-resize dividers. Must be added as a new dependency.

## Data Model

No new backend tables. All state is frontend-only.

### Zustand Store: `multiviewStore`

```typescript
interface MultiviewState {
  workspaces: Workspace[]
  activeWorkspace: Workspace | null
  focusedPaneId: string | null
}

interface Workspace {
  id: string              // crypto.randomUUID()
  name: string | null     // null = unsaved scratch workspace
  layout: LayoutConfig
  panes: Pane[]           // 2-6 entries
  createdAt: string
  updatedAt: string
}

interface Pane {
  id: string              // unique within workspace
  sessionId: string       // references existing session
}

interface LayoutConfig {
  preset: LayoutPreset
  autoSaveId?: string     // react-resizable-panels persistence key (handles nested sizes internally)
}

type LayoutPreset =
  | '2-horizontal' | '2-vertical'
  | '3-columns' | '3-main-side'
  | '4-grid' | '6-grid'
  | 'custom'
```

### Persistence

- Saved workspaces: `localStorage` key `claude-plane:multiview:workspaces`
- Scratch workspace: `localStorage` key `claude-plane:multiview:scratch`
- Per-browser storage (UI preference, not shared state)

## Routing & Entry Points

### Routes

- `/multiview` — loads scratch workspace, or empty state on first visit
- `/multiview/:workspaceId` — loads a specific saved workspace

### Entry Points

1. **Sidebar link** — "Multi-View" item with `LayoutGrid` or `PanelsTopLeft` icon, positioned between Sessions and Machines
2. **Sessions page multi-select** — Checkboxes on session cards; selecting 2+ shows "Open in Multi-View" floating toolbar button. Clicking writes selected session IDs into the multiview Zustand store as a new scratch workspace, then navigates to `/multiview`. The store (persisted to localStorage) is the source of truth — no URL query params or router state needed.
3. **Inside multi-view** — Session picker per pane (swap, add, remove sessions in-place)

### Navigation Behavior

- Navigating away does not kill WebSocket connections or terminal state (Zustand store preserves configuration)
- Returning to multi-view remounts xterm.js instances which replay scrollback (existing behavior)

## Layout Engine

### Panel Structure

All layouts are trees of nested `PanelGroup` components from `react-resizable-panels`:

| Preset | Structure |
|--------|-----------|
| 2-horizontal | `H[Panel, Panel]` |
| 2-vertical | `V[Panel, Panel]` |
| 3-columns | `H[Panel, Panel, Panel]` |
| 3-main-side | `H[Panel(66%), V[Panel, Panel]]` |
| 4-grid | `V[H[Panel, Panel], H[Panel, Panel]]` |
| 6-grid | `V[H[Panel, Panel, Panel], H[Panel, Panel, Panel]]` |
| custom | Starts as 2-horizontal; user adds/removes panes and drags dividers |

### Constraints

- Minimum pane width: 200px
- Minimum pane height: 150px
- Enforced as percentage-based min sizes calculated from container dimensions

### Panel Size Persistence

`react-resizable-panels` has built-in persistence via `autoSaveId`. Each `PanelGroup` is assigned an `autoSaveId` derived from the workspace ID (e.g., `multiview-{workspaceId}-outer`, `multiview-{workspaceId}-inner-0`). The library handles nested size serialization internally — no custom `panelSizes` tracking needed.

### Pane-to-Position Mapping

Panes map to layout positions in reading order (left-to-right, top-to-bottom). For `3-main-side`: pane[0] = main left panel, pane[1] = top-right, pane[2] = bottom-right. `Ctrl+Shift+1` always refers to the first pane in reading order.

### Layout Transitions on Pane Add/Remove

- **Adding a pane:** Layout switches to the natural preset for the new count (3 panes → `3-columns`, 4 → `4-grid`, 5 → 3-top + 2-bottom (`V[H[P,P,P], H[P,P]]`), 6 → `6-grid`). User can change the preset after.
- **Removing a pane:** Same logic — transitions to the natural preset for the reduced count. If the removed pane was focused, focus moves to the previous pane in reading order.

### Resize Flow

1. User drags a divider
2. `react-resizable-panels` updates panel percentages
3. Each pane's `ResizeObserver` (existing in `useTerminalSession`) fires
4. `fitAddon.fit()` recalculates terminal columns/rows
5. Resize control message sent over WebSocket to agent
6. Agent resizes PTY — output reflows

### Layout Picker Toolbar

Top of multi-view page:
- Layout preset icons (visual thumbnails of each grid shape)
- Workspace name (editable inline) + save button
- Workspace switcher dropdown
- "Add pane" button (if under 6 panes)

## Focus Management & Input Routing

### Focus Model

- Exactly one pane is "focused" at any time (or none if clicking outside panes)
- Clicking inside a pane's terminal area sets focus
- Focused pane's xterm.js instance receives all keyboard input
- Unfocused panes remain live (output streams) but don't capture keystrokes

### Visual Focus Indicator

- **Focused pane:** 2px accent border (app's blue/purple accent) + subtle glow shadow
- **Unfocused panes:** 1px neutral border (muted gray)
- Instant transition — no animation delay

### Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+Shift+ArrowKey` | Move focus to adjacent pane |
| `Ctrl+Shift+1-6` | Jump focus to pane by number |
| `Escape` | Unfocus all panes (access toolbar) |
| `Ctrl+Shift+M` | Toggle maximize on focused pane |

`Ctrl+Shift` prefix avoids conflicts with terminal shortcuts (`Ctrl+C`, `Ctrl+D`) and Claude CLI shortcuts.

### Arrow Key Navigation in Non-Grid Layouts

For non-uniform layouts like `3-main-side`, arrow key navigation follows spatial proximity. If no pane exists in the pressed direction (e.g., pressing Down from the main left pane in `3-main-side`), the command is a no-op — no wrapping. This keeps behavior predictable. `Ctrl+Shift+1-6` provides an unambiguous alternative.

### Edge Cases

- Terminated session: pane shows final terminal output with "Session ended" overlay; focus does not auto-jump
- Disconnected session: existing `useTerminalSession` reconnection logic handles transparently
- All sessions stale: when every pane in a workspace shows "Session no longer available," display a full-page empty state: "All sessions in this workspace have ended" with options to pick new sessions or delete the workspace

## Pane Header

Always-visible, ~24px tall, dark background slightly lighter than terminal:

```
[Session type icon] | machine-name | ~/path/to/dir     [⤢]
```

- **Left:** Claude sparkle icon or Terminal `>_` icon
- **Middle:** Machine display name + working directory (truncated from left with ellipsis: `…/src/components`). Machine names are resolved from `machine_id` using the existing `useMachines()` hook — the multi-view page fetches machines once and passes a lookup map to pane headers.
- **Right:** Maximize button (Lucide `Maximize2`, 14px, subtle until hovered)

### Maximize Behavior

- Click maximize button → pane fills entire multi-view area, others hidden
- Button changes to `Minimize2` (restore)
- Click restore → returns to previous grid layout with exact split ratios
- While maximized, `Ctrl+Shift+ArrowKey` navigation disabled

### Pane Context Menu (right-click on header only)

- "Swap session..." — opens session picker
- "Remove pane" — removes pane (minimum 2 panes enforced)
- "Open in full view" — navigates to `/sessions/:id`

Right-click on terminal area left untouched for browser/terminal default behavior.

## Session Picker

Modal/dropdown with search input, used for adding panes, swapping sessions, or initial setup:

- Shows running sessions by default, grouped by machine
- Each row: session type icon, session ID (truncated), working directory, machine name, status badge
- Sessions already in multi-view shown grayed out with "Already in view" label
- Filterable by machine and session type

## Workspace Management

- **Save:** Click save icon next to workspace name; prompts for name if unnamed
- **Save As:** Dropdown option next to save; creates copy with new name
- **Rename:** Click workspace name text to edit inline
- **Delete:** Trash icon per workspace in switcher dropdown, with confirmation
- **Workspace switcher:** Dropdown listing saved workspaces + "New workspace"
- **Scratch behavior:** Unsaved modifications auto-persist as scratch; saved workspaces untouched until explicit save
- **Stale sessions:** If a saved workspace references a terminated/deleted session, pane shows empty state with "Session no longer available" and session picker

## Performance Considerations

### WebGL Context Budget

Browsers limit concurrent WebGL contexts (~16 in Chrome, fewer in some browsers). With 6 panes each using the WebGL addon, that's 6 contexts from this page alone plus any other tabs. Strategy:

- **4 or fewer panes:** Use WebGL addon (current behavior, best rendering performance)
- **5-6 panes:** Fall back to the default canvas renderer (no WebGL addon loaded). Canvas performance is sufficient for terminal rendering at smaller pane sizes.
- The `useTerminalSession` hook should accept an optional `useWebGL` parameter (default `true`) that the multi-view page sets based on pane count.

### Resize Debouncing

The existing `ResizeObserver` in `useTerminalSession` fires on every pixel during drag-resize. For multi-view with 6 panes resizing simultaneously:

- Debounce `fitAddon.fit()` calls with a 50ms trailing debounce during active resize
- Send the WebSocket resize control message only after the debounce settles (avoids flooding the agent with PTY resize calls)

### Loading State

When a workspace loads with multiple sessions, all terminals enter "connecting" state simultaneously. Each pane independently shows the existing connecting → replaying → live status indicator. No additional page-level loading state is needed — the per-pane indicators are sufficient and familiar.

## Testing Strategy

### Unit Tests
- Zustand store: workspace CRUD, layout switching, focus management, localStorage persistence
- Layout config generation: preset-to-panel-structure mapping
- Pane header: metadata display, truncation logic

### Integration Tests
- Multi-view page render with mock sessions
- Session picker filtering and selection
- Layout preset switching
- Focus cycling via keyboard shortcuts
- Maximize/restore flow

### E2E Tests
- Create multi-view from sessions page multi-select
- Switch layouts, verify terminals resize
- Save workspace, navigate away, return and verify restoration
- Swap a session in a pane
- Focus management: click-to-focus, keyboard navigation
