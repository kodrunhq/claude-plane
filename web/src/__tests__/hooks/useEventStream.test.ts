import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { useEventStream } from '../../hooks/useEventStream.ts';
import { useRunStore } from '../../stores/runs.ts';

// ---------------------------------------------------------------------------
// Mock WebSocket
// ---------------------------------------------------------------------------

interface MockWs {
  onopen: (() => void) | null;
  onmessage: ((ev: { data: string }) => void) | null;
  onclose: (() => void) | null;
  onerror: (() => void) | null;
  close: ReturnType<typeof vi.fn>;
  readyState: number;
}

// Holds a reference to the most recently created WebSocket instance so tests
// can invoke its handlers (onopen, onmessage, etc.).
let mockWs: MockWs;

class MockWebSocket {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  readyState = 1;

  constructor() {
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    mockWs = this as unknown as MockWs;
  }
}
vi.stubGlobal('WebSocket', MockWebSocket);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function buildEventMsg(eventType: string, payload: Record<string, unknown> = {}) {
  return JSON.stringify({
    type: 'event',
    event_id: 'evt-1',
    event_type: eventType,
    timestamp: new Date().toISOString(),
    source: 'test',
    payload,
  });
}

function sendEvent(eventType: string, payload: Record<string, unknown> = {}) {
  mockWs.onmessage?.({ data: buildEventMsg(eventType, payload) });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useEventStream', () => {
  let queryClient: QueryClient;
  let invalidateSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    invalidateSpy = vi.fn();
    queryClient.invalidateQueries = invalidateSpy;
    useRunStore.getState().reset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  function renderStream() {
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);

    const result = renderHook(() => useEventStream(), { wrapper });

    // Simulate WebSocket open
    act(() => {
      mockWs.onopen?.();
    });

    return result;
  }

  // --- Correct event types ---------------------------------------------------

  it('invalidates machines query on machine.connected', () => {
    renderStream();
    act(() => sendEvent('machine.connected', { machine_id: 'm1' }));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['machines'] });
  });

  it('invalidates machines query on machine.disconnected', () => {
    renderStream();
    act(() => sendEvent('machine.disconnected', { machine_id: 'm1' }));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['machines'] });
  });

  it('invalidates sessions query on session.started', () => {
    renderStream();
    act(() => sendEvent('session.started', { session_id: 's1' }));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['sessions'] });
  });

  it('invalidates runs query on run.completed', () => {
    renderStream();
    act(() => sendEvent('run.completed', { run_id: 'r1' }));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['runs'] });
  });

  it('invalidates templates query on template.created', () => {
    renderStream();
    act(() => sendEvent('template.created', { template_id: 't1' }));
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['templates'] });
  });

  it('calls updateTaskStatus with snake_case fields on run.step.completed', () => {
    const updateSpy = vi.fn();
    useRunStore.setState({ updateTaskStatus: updateSpy });

    renderStream();
    act(() =>
      sendEvent('run.step.completed', {
        run_id: 'r1',
        step_id: 's1',
        status: 'completed',
        session_id: 'sess-1',
      }),
    );

    expect(updateSpy).toHaveBeenCalledWith('r1', 's1', 'completed', 'sess-1');
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['runs'] });
  });

  it('calls updateTaskStatus on run.step.failed', () => {
    const updateSpy = vi.fn();
    useRunStore.setState({ updateTaskStatus: updateSpy });

    renderStream();
    act(() =>
      sendEvent('run.step.failed', {
        run_id: 'r2',
        step_id: 's2',
        status: 'failed',
      }),
    );

    expect(updateSpy).toHaveBeenCalledWith('r2', 's2', 'failed', undefined);
  });

  // --- Old (wrong) event types must NOT trigger actions ----------------------

  it('does NOT act on old machine.status event', () => {
    renderStream();
    act(() => sendEvent('machine.status', { machine_id: 'm1' }));
    expect(invalidateSpy).not.toHaveBeenCalled();
  });

  it('does NOT act on old machine.health event', () => {
    renderStream();
    act(() => sendEvent('machine.health', { machine_id: 'm1' }));
    expect(invalidateSpy).not.toHaveBeenCalled();
  });

  it('does NOT act on old run.step.status event', () => {
    const updateSpy = vi.fn();
    useRunStore.setState({ updateTaskStatus: updateSpy });

    renderStream();
    act(() =>
      sendEvent('run.step.status', { runId: 'r1', stepId: 's1', status: 'completed' }),
    );

    expect(updateSpy).not.toHaveBeenCalled();
    expect(invalidateSpy).not.toHaveBeenCalled();
  });
});
