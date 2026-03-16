import { create } from 'zustand';
import type { Workspace, Pane, LayoutPreset } from '../types/multiview';

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

function loadWorkspacesFromStorage(): Workspace[] {
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
  workspaces: loadWorkspacesFromStorage(),
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
