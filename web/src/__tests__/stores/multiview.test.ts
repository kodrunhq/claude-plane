import { describe, it, expect, beforeEach, vi } from 'vitest';

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

  it('should shift focus to previous pane when removing the focused pane', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, setFocusedPane, removePane } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2', 's3']);
    const panes = useMultiviewStore.getState().activeWorkspace!.panes;
    setFocusedPane(panes[1].id);
    removePane(panes[1].id);

    expect(useMultiviewStore.getState().focusedPaneId).toBe(panes[0].id);
  });

  it('should clear activeWorkspace and scratch on deleteWorkspace when active', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, saveWorkspace, deleteWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    saveWorkspace('Active WS');
    const wsId = useMultiviewStore.getState().workspaces[0].id;
    deleteWorkspace(wsId);

    expect(useMultiviewStore.getState().activeWorkspace).toBeNull();
    expect(localStorage.getItem('claude-plane:multiview:scratch')).toBeNull();
  });

  it('should not change activeWorkspace when loading non-existent workspace', async () => {
    const { useMultiviewStore } = await import('../../stores/multiview');
    const { createScratchWorkspace, loadWorkspace } = useMultiviewStore.getState();

    createScratchWorkspace(['s1', 's2']);
    const before = useMultiviewStore.getState().activeWorkspace;
    loadWorkspace('non-existent-id');

    expect(useMultiviewStore.getState().activeWorkspace).toBe(before);
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
