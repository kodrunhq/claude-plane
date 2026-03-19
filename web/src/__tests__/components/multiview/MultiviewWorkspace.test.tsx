import { describe, it, expect, beforeEach } from 'vitest';
import { generateUUID } from '../../../lib/uuid';
import { useMultiviewStore } from '../../../stores/multiview';

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

describe('generateUUID', () => {
  it('returns a string matching the UUID v4 format', () => {
    const uuid = generateUUID();
    expect(uuid).toMatch(UUID_RE);
  });

  it('generates unique values on successive calls', () => {
    const a = generateUUID();
    const b = generateUUID();
    expect(a).not.toBe(b);
  });
});

describe('multiview store', () => {
  beforeEach(() => {
    // Reset the store and localStorage between tests
    localStorage.clear();
    useMultiviewStore.setState({
      workspaces: [],
      activeWorkspace: null,
      focusedPaneId: null,
    });
  });

  describe('createScratchWorkspace', () => {
    it('creates a workspace with the correct number of panes for 2 sessions', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['sess-a', 'sess-b']);

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace).not.toBeNull();
      expect(activeWorkspace!.panes).toHaveLength(2);
      expect(activeWorkspace!.panes[0].sessionId).toBe('sess-a');
      expect(activeWorkspace!.panes[1].sessionId).toBe('sess-b');
    });

    it('creates a workspace with the correct number of panes for 4 sessions', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['s1', 's2', 's3', 's4']);

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace!.panes).toHaveLength(4);
      expect(activeWorkspace!.layout.preset).toBe('4-grid');
    });

    it('persists the workspace to localStorage', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['sess-a', 'sess-b']);

      const stored = localStorage.getItem('claude-plane:multiview:scratch');
      expect(stored).not.toBeNull();
      const parsed = JSON.parse(stored!);
      expect(parsed.panes).toHaveLength(2);
      expect(parsed.id).toMatch(UUID_RE);
    });

    it('clamps to 6 panes maximum', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['1', '2', '3', '4', '5', '6', '7', '8']);

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace!.panes).toHaveLength(6);
    });
  });

  describe('addPane', () => {
    it('adds a pane to an existing workspace', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['sess-a', 'sess-b']);

      useMultiviewStore.getState().addPane('sess-c');

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace!.panes).toHaveLength(3);
      expect(activeWorkspace!.panes[2].sessionId).toBe('sess-c');
    });

    it('does not exceed 6 panes', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['1', '2', '3', '4', '5', '6']);

      useMultiviewStore.getState().addPane('7');

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace!.panes).toHaveLength(6);
    });
  });

  describe('removePane', () => {
    // Bug regression: must prevent going below 2 panes.
    it('prevents removing a pane when only 2 panes remain', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['sess-a', 'sess-b']);

      const paneId = useMultiviewStore.getState().activeWorkspace!.panes[0].id;
      useMultiviewStore.getState().removePane(paneId);

      const { activeWorkspace } = useMultiviewStore.getState();
      // Should still have 2 panes — removal was blocked
      expect(activeWorkspace!.panes).toHaveLength(2);
    });

    it('allows removing a pane when 3 or more panes exist', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['sess-a', 'sess-b', 'sess-c']);

      const paneId = useMultiviewStore.getState().activeWorkspace!.panes[1].id;
      useMultiviewStore.getState().removePane(paneId);

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace!.panes).toHaveLength(2);
    });
  });

  describe('swapSession', () => {
    it('updates the session ID on the correct pane', () => {
      const { createScratchWorkspace } = useMultiviewStore.getState();
      createScratchWorkspace(['sess-a', 'sess-b', 'sess-c']);

      const targetPane = useMultiviewStore.getState().activeWorkspace!.panes[1];
      useMultiviewStore.getState().swapSession(targetPane.id, 'sess-new');

      const { activeWorkspace } = useMultiviewStore.getState();
      expect(activeWorkspace!.panes[1].sessionId).toBe('sess-new');
      // Other panes remain unchanged
      expect(activeWorkspace!.panes[0].sessionId).toBe('sess-a');
      expect(activeWorkspace!.panes[2].sessionId).toBe('sess-c');
    });
  });
});
