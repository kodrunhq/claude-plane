import { useEffect, useRef, useState, useCallback } from 'react';
import type { RefObject } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';
import type { TerminalStatus } from '../types/session.ts';

export type { TerminalStatus };

/** Terminal statuses that represent a final state — ws.onclose must not override them. */
const TERMINAL_STATUSES: ReadonlySet<TerminalStatus> = new Set(['ended', 'agent_offline']);

export function useTerminalSession(
  sessionId: string,
  containerRef: RefObject<HTMLDivElement | null>,
  options?: { useWebGL?: boolean; fontSize?: number },
) {
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [status, setStatus] = useState<TerminalStatus>('connecting');
  const [showScrollButton, setShowScrollButton] = useState(false);
  const [containerEl, setContainerEl] = useState<HTMLDivElement | null>(null);

  // Keep a stable reference to the actual container element so the effect
  // re-runs if the underlying DOM node changes.
  useEffect(() => {
    setContainerEl(containerRef.current);
  }, [containerRef]);

  useEffect(() => {
    if (!containerEl || !sessionId) return;

    // Validate sessionId format before constructing WebSocket URL
    const uuidRe = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    if (!uuidRe.test(sessionId)) {
      setStatus('disconnected');
      return;
    }

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'JetBrains Mono, Menlo, Monaco, Consolas, monospace',
      fontSize: options?.fontSize ?? 14,
      scrollback: 10000,
      theme: {
        background: '#1a1b26',
        foreground: '#c0caf5',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    if (options?.useWebGL !== false) {
      try {
        term.loadAddon(new WebglAddon());
      } catch {
        // Falls back to canvas/DOM renderer silently
      }
    }

    term.open(containerEl);

    // --- Scroll pinning logic ---
    const isScrollPinned = { current: true };
    let scrollDebounceTimer: number | undefined;
    let scrollRafId: number | undefined;

    const onScrollDisposable = term.onScroll(() => {
      const buf = term.buffer.active;
      const pinned = buf.viewportY >= buf.baseY;
      isScrollPinned.current = pinned;

      clearTimeout(scrollDebounceTimer);
      scrollDebounceTimer = window.setTimeout(() => {
        setShowScrollButton(!pinned);
      }, 100);
    });

    const scrollIfPinned = () => {
      if (!isScrollPinned.current) return;
      if (scrollRafId != null) return; // already scheduled
      scrollRafId = requestAnimationFrame(() => {
        scrollRafId = undefined;
        term.scrollToBottom();
      });
    };

    // Initial fit deferred to next frame so the container has layout dimensions.
    const initialFitFrame = requestAnimationFrame(() => {
      fitAddon.fit();
      scrollIfPinned();
    });

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // Track whether we've received session_ended so scrollback_end can
    // transition directly to the correct final state instead of 'live'.
    let receivedSessionEnded = false;
    // Track the status field from session_ended to distinguish agent-offline
    // from normal session termination.
    let sessionEndedStatus = '';

    // WebSocket connection
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/ws/terminal/${sessionId}`,
    );
    ws.binaryType = 'arraybuffer';

    let scrollbackTimeout: number | undefined;

    setStatus('connecting');

    ws.onopen = () => {
      setStatus('replaying');

      // Always send the current terminal dimensions on connect. The initial
      // fitAddon.fit() (on RAF) fires before the WS is open, so term.onResize
      // drops the message (readyState !== OPEN). By the time ws.onopen fires,
      // fit() is a no-op because xterm already has the correct size — but the
      // server/agent never received it. Send it explicitly here.
      fitAddon.fit();
      scrollIfPinned();
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));

      // Safety timeout: if scrollback_end never arrives, transition to live
      scrollbackTimeout = window.setTimeout(() => {
        setStatus((prev) => (prev === 'replaying' ? 'live' : prev));
      }, 10_000);
    };

    ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
        scrollIfPinned();
      } else {
        try {
          const msg = JSON.parse(event.data as string) as { type: string; status?: string };
          if (msg.type === 'scrollback_end') {
            clearTimeout(scrollbackTimeout);
            // If session_ended already arrived (dead session replay), go
            // straight to final state instead of transitioning to 'live'.
            if (receivedSessionEnded) {
              setStatus(sessionEndedStatus === 'disconnected' ? 'agent_offline' : 'ended');
            } else {
              setStatus('live');
            }
          } else if (msg.type === 'session_ended') {
            receivedSessionEnded = true;
            sessionEndedStatus = msg.status ?? '';
            // If the agent couldn't serve scrollback (offline), distinguish
            // from a normal session end.
            if (msg.status === 'disconnected') {
              setStatus('agent_offline');
            } else {
              setStatus('ended');
            }
          }
        } catch {
          // Ignore unparseable control messages
        }
      }
    };

    ws.onerror = () => {
      setStatus('disconnected');
    };

    ws.onclose = () => {
      // Only transition to 'disconnected' if not already in a terminal state.
      // The session_ended control message arrives before ws.onclose for dead
      // sessions — we must not override 'ended' or 'agent_offline'.
      setStatus((prev) => (TERMINAL_STATUSES.has(prev) ? prev : 'disconnected'));
    };

    wsRef.current = ws;

    // Keystrokes -> server (binary frames).
    // Guard against sending to ended sessions — the PTY is dead.
    const onDataDisposable = term.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN && !receivedSessionEnded) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    // Resize -> server (JSON control message)
    const onResizeDisposable = term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });

    // Container resize -> fit terminal (debounced)
    let resizeTimer: ReturnType<typeof setTimeout> | null = null;
    const observer = new ResizeObserver(() => {
      if (resizeTimer) clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        fitAddon.fit();
        scrollIfPinned();
      }, 50);
    });
    observer.observe(containerEl);

    return () => {
      if (resizeTimer) clearTimeout(resizeTimer);
      cancelAnimationFrame(initialFitFrame);
      clearTimeout(scrollbackTimeout);
      clearTimeout(scrollDebounceTimer);
      if (scrollRafId != null) cancelAnimationFrame(scrollRafId);
      onScrollDisposable.dispose();
      onDataDisposable.dispose();
      onResizeDisposable.dispose();
      observer.disconnect();
      ws.onopen = null;
      ws.onmessage = null;
      ws.onerror = null;
      ws.onclose = null;
      ws.close();
      term.dispose();
      termRef.current = null;
      wsRef.current = null;
      fitAddonRef.current = null;
    };
  }, [sessionId, containerEl, options?.useWebGL, options?.fontSize]);

  const fitTerminal = useCallback(() => {
    fitAddonRef.current?.fit();
  }, []);

  const focusTerminal = useCallback(() => {
    termRef.current?.focus();
  }, []);

  const scrollToBottom = useCallback(() => {
    termRef.current?.scrollToBottom();
  }, []);

  return { status, showScrollButton, term: termRef, ws: wsRef, fitTerminal, focusTerminal, scrollToBottom };
}
