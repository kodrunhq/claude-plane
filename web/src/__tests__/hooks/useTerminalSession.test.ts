import { describe, it, expect, vi, beforeAll, beforeEach, afterAll } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useTerminalSession } from '../../hooks/useTerminalSession.ts';
import type { TerminalStatus } from '../../types/session.ts';
import type { RefObject } from 'react';

// ---------------------------------------------------------------------------
// Mock WebSocket
// ---------------------------------------------------------------------------

interface MockWs {
  url: string;
  binaryType: string;
  onopen: (() => void) | null;
  onmessage: ((ev: { data: string | ArrayBuffer }) => void) | null;
  onclose: (() => void) | null;
  onerror: (() => void) | null;
  close: ReturnType<typeof vi.fn>;
  send: ReturnType<typeof vi.fn>;
  readyState: number;
}

let mockWs: MockWs;

class MockWebSocket {
  url: string;
  binaryType = 'blob';
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string | ArrayBuffer }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  send = vi.fn();
  readyState = 1;

  static OPEN = 1;

  constructor(url: string) {
    this.url = url;
    mockWs = this as unknown as MockWs;
  }
}

// ---------------------------------------------------------------------------
// Mock xterm.js + addons
// ---------------------------------------------------------------------------

const mockTermOpen = vi.fn();
const mockTermWrite = vi.fn();
const mockTermDispose = vi.fn();
const mockTermFocus = vi.fn();
const mockTermOnData = vi.fn(() => ({ dispose: vi.fn() }));
const mockTermOnResize = vi.fn(() => ({ dispose: vi.fn() }));

vi.mock('@xterm/xterm', () => {
  class MockTerminal {
    open = mockTermOpen;
    write = mockTermWrite;
    dispose = mockTermDispose;
    focus = mockTermFocus;
    onData = mockTermOnData;
    onResize = mockTermOnResize;
    loadAddon = vi.fn();
    cols = 80;
    rows = 24;
  }
  return { Terminal: MockTerminal };
});

vi.mock('@xterm/addon-fit', () => {
  class MockFitAddon {
    fit = vi.fn();
  }
  return { FitAddon: MockFitAddon };
});

vi.mock('@xterm/addon-webgl', () => {
  class MockWebglAddon {}
  return { WebglAddon: MockWebglAddon };
});

vi.mock('@xterm/xterm/css/xterm.css', () => ({}));

// ---------------------------------------------------------------------------
// Globals
// ---------------------------------------------------------------------------

let savedWebSocket: typeof globalThis.WebSocket;

beforeAll(() => {
  savedWebSocket = globalThis.WebSocket;
  vi.stubGlobal('WebSocket', MockWebSocket);
  // Mock requestAnimationFrame
  vi.stubGlobal('requestAnimationFrame', (cb: () => void) => { cb(); return 0; });
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
  // Mock ResizeObserver
  vi.stubGlobal('ResizeObserver', class {
    observe = vi.fn();
    disconnect = vi.fn();
    unobserve = vi.fn();
  });
});

afterAll(() => {
  globalThis.WebSocket = savedWebSocket;
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const VALID_SESSION_ID = '12345678-1234-1234-1234-123456789abc';

function makeContainerRef(): RefObject<HTMLDivElement> {
  const div = document.createElement('div');
  return { current: div };
}

function sendControlMessage(type: string, extra: Record<string, string> = {}) {
  mockWs.onmessage?.({ data: JSON.stringify({ type, ...extra }) });
}

function fireWsOpen() {
  mockWs.onopen?.();
}

function fireWsClose() {
  mockWs.onclose?.();
}

function fireWsError() {
  mockWs.onerror?.();
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useTerminalSession', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  describe('initial state', () => {
    it('starts in connecting status', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );
      expect(result.current.status).toBe('connecting');
    });

    it('transitions to replaying on ws.onopen', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      expect(result.current.status).toBe('replaying');
    });

    it('rejects invalid session IDs', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession('not-a-uuid', containerRef),
      );
      expect(result.current.status).toBe('disconnected');
    });
  });

  describe('live session flow', () => {
    it('transitions replaying → live on scrollback_end', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      expect(result.current.status).toBe('replaying');

      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('live');
    });

    it('transitions live → ended on session_ended', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('live');

      act(() => sendControlMessage('session_ended'));
      expect(result.current.status).toBe('ended');
    });

    it('transitions live → disconnected on ws.onclose', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('live');

      act(() => fireWsClose());
      expect(result.current.status).toBe('disconnected');
    });
  });

  describe('dead session replay (key fix)', () => {
    it('does NOT override ended with disconnected on ws.onclose', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      // Dead session: server sends session_ended before scrollback_end
      act(() => sendControlMessage('session_ended'));
      expect(result.current.status).toBe('ended');

      // WebSocket closes after sending control messages
      act(() => fireWsClose());
      // Must remain 'ended', NOT 'disconnected'
      expect(result.current.status).toBe('ended');
    });

    it('handles scrollback_end after session_ended → transitions to ended', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      // session_ended arrives first
      act(() => sendControlMessage('session_ended'));
      expect(result.current.status).toBe('ended');

      // scrollback_end arrives after — should stay ended
      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('ended');

      // ws close — still ended
      act(() => fireWsClose());
      expect(result.current.status).toBe('ended');
    });

    it('handles scrollback_end before session_ended for dead sessions', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      // scrollback_end arrives first (transition to live briefly)
      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('live');

      // session_ended arrives → transitions to ended
      act(() => sendControlMessage('session_ended'));
      expect(result.current.status).toBe('ended');

      // ws close — must remain ended
      act(() => fireWsClose());
      expect(result.current.status).toBe('ended');
    });
  });

  describe('agent offline detection', () => {
    it('transitions to agent_offline when session_ended has status=disconnected', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('session_ended', { status: 'disconnected' }));
      expect(result.current.status).toBe('agent_offline');
    });

    it('does NOT override agent_offline with disconnected on ws.onclose', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('session_ended', { status: 'disconnected' }));
      expect(result.current.status).toBe('agent_offline');

      act(() => fireWsClose());
      expect(result.current.status).toBe('agent_offline');
    });

    it('scrollback_end after session_ended with status=disconnected → agent_offline', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('session_ended', { status: 'disconnected' }));
      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('agent_offline');
    });
  });

  describe('input guard for ended sessions', () => {
    it('does not send keystrokes after session_ended', () => {
      const containerRef = makeContainerRef();
      renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('session_ended'));

      // Get the onData callback that was registered with the terminal
      const onDataCallback = mockTermOnData.mock.calls[0]?.[0] as ((data: string) => void) | undefined;
      expect(onDataCallback).toBeDefined();

      // Simulate a keystroke — it should NOT be sent via ws
      mockWs.send.mockClear();
      onDataCallback?.('x');
      expect(mockWs.send).not.toHaveBeenCalled();
    });
  });

  describe('error handling', () => {
    it('transitions to disconnected on ws.onerror', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => fireWsError());
      expect(result.current.status).toBe('disconnected');
    });

    it('safety timeout transitions replaying → live after 10s', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      expect(result.current.status).toBe('replaying');

      // Advance 10 seconds
      act(() => { vi.advanceTimersByTime(10_000); });
      expect(result.current.status).toBe('live');
    });

    it('safety timeout does not fire if scrollback_end arrives first', () => {
      const containerRef = makeContainerRef();
      const { result } = renderHook(() =>
        useTerminalSession(VALID_SESSION_ID, containerRef),
      );

      act(() => fireWsOpen());
      act(() => sendControlMessage('scrollback_end'));
      expect(result.current.status).toBe('live');

      // Simulate session_ended to move to terminal state
      act(() => sendControlMessage('session_ended'));
      expect(result.current.status).toBe('ended');

      // Even after 10s, status should stay ended
      act(() => { vi.advanceTimersByTime(10_000); });
      expect(result.current.status).toBe('ended');
    });
  });

  describe('status transition table', () => {
    const scenarios: Array<{
      name: string;
      events: Array<() => void>;
      expected: TerminalStatus;
    }> = [
      {
        name: 'connecting → replaying → live → ended → (ws close) = ended',
        events: [fireWsOpen, () => sendControlMessage('scrollback_end'), () => sendControlMessage('session_ended'), fireWsClose],
        expected: 'ended',
      },
      {
        name: 'connecting → replaying → session_ended → (ws close) = ended',
        events: [fireWsOpen, () => sendControlMessage('session_ended'), fireWsClose],
        expected: 'ended',
      },
      {
        name: 'connecting → replaying → session_ended(disconnected) → (ws close) = agent_offline',
        events: [fireWsOpen, () => sendControlMessage('session_ended', { status: 'disconnected' }), fireWsClose],
        expected: 'agent_offline',
      },
      {
        name: 'connecting → replaying → live → (ws close) = disconnected',
        events: [fireWsOpen, () => sendControlMessage('scrollback_end'), fireWsClose],
        expected: 'disconnected',
      },
      {
        name: 'connecting → (ws error) = disconnected',
        events: [fireWsError],
        expected: 'disconnected',
      },
    ];

    for (const scenario of scenarios) {
      it(scenario.name, () => {
        const containerRef = makeContainerRef();
        const { result } = renderHook(() =>
          useTerminalSession(VALID_SESSION_ID, containerRef),
        );

        for (const event of scenario.events) {
          act(() => event());
        }

        expect(result.current.status).toBe(scenario.expected);
      });
    }
  });
});
