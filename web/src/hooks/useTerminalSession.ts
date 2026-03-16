import { useEffect, useRef, useState, useCallback } from 'react';
import type { RefObject } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';
import type { TerminalStatus } from '../types/session.ts';

export type { TerminalStatus };

export function useTerminalSession(
  sessionId: string,
  containerRef: RefObject<HTMLDivElement | null>,
  options?: { useWebGL?: boolean; fontSize?: number },
) {
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [status, setStatus] = useState<TerminalStatus>('connecting');
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

    // 1. Create xterm.js instance
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

    // WebGL renderer with silent fallback to canvas/DOM
    if (options?.useWebGL !== false) {
      try {
        term.loadAddon(new WebglAddon());
      } catch {
        // Falls back to canvas/DOM renderer silently
      }
    }

    term.open(containerEl);

    // Staggered fit strategy: xterm.js needs the container to have its final
    // CSS dimensions before fit() can calculate correct rows/cols. In complex
    // layouts (multi-view split panes, resizable panels), the container may not
    // reach its final size until several frames after mount. We fit at multiple
    // intervals to catch different layout timing scenarios:
    //   - RAF: catches immediate layout (simple cases)
    //   - 150ms: catches CSS transitions and panel layout calculations
    //   - 500ms: catches slow layout engines and deferred rendering
    // Subsequent fits are no-ops if dimensions haven't changed, so the overhead
    // is negligible.
    const fitTimers: ReturnType<typeof setTimeout>[] = [];
    const initialFitFrame = requestAnimationFrame(() => {
      fitAddon.fit();
    });
    fitTimers.push(setTimeout(() => fitAddon.fit(), 150));
    fitTimers.push(setTimeout(() => fitAddon.fit(), 500));

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // 2. WebSocket connection (first-message auth)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/ws/terminal/${sessionId}`,
    );
    ws.binaryType = 'arraybuffer';

    let scrollbackTimeout: number | undefined;

    setStatus('connecting');

    ws.onopen = () => {
      // Cookie-based auth: the session_token cookie is sent automatically
      // on the WebSocket upgrade request, so no first-message auth is needed.

      // CRITICAL: send resize BEFORE anything else. The server will forward
      // this to the agent before the attach command's scrollback replay.
      // Without this, scrollback is replayed at the wrong terminal size,
      // causing garbled rendering (Claude CLI's status bar, horizontal rules,
      // and cursor positioning all depend on correct column count).
      fitAddon.fit();

      setStatus('replaying');

      // Safety timeout: if scrollback_end never arrives, transition to live
      // mode after 10 seconds to avoid being stuck on "Loading history...".
      scrollbackTimeout = window.setTimeout(() => {
        setStatus((prev) => (prev === 'replaying' ? 'live' : prev));
      }, 10_000);
    };

    ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        // Binary frame: terminal output (scrollback or live)
        term.write(new Uint8Array(event.data));
      } else {
        // Text frame: control message
        try {
          const msg = JSON.parse(event.data as string) as { type: string };
          if (msg.type === 'scrollback_end') {
            clearTimeout(scrollbackTimeout);
            setStatus('live');
            // Clear the terminal buffer and re-fit. Scrollback was replayed at
            // whatever size the PTY had when the data was generated (often the
            // default 80x24 or a different browser width). Full-screen TUI apps
            // like Claude CLI use cursor positioning and full-width rules that
            // look garbled at the wrong size. Clearing wipes the stale output,
            // and the fit sends a resize → SIGWINCH which makes the CLI redraw
            // its entire screen cleanly at the correct dimensions.
            term.clear();
            requestAnimationFrame(() => fitAddon.fit());
          } else if (msg.type === 'session_ended') {
            setStatus('disconnected');
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
      setStatus('disconnected');
    };

    wsRef.current = ws;

    // 3. Keystrokes -> server (binary frames)
    const onDataDisposable = term.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    // 4. Resize -> server (JSON control message)
    const onResizeDisposable = term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });

    // 5. Container resize -> fit terminal (debounced for multi-pane performance)
    let resizeTimer: ReturnType<typeof setTimeout> | null = null;
    const observer = new ResizeObserver(() => {
      if (resizeTimer) clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        fitAddon.fit();
      }, 50);
    });
    observer.observe(containerEl);

    // Cleanup
    return () => {
      if (resizeTimer) clearTimeout(resizeTimer);
      cancelAnimationFrame(initialFitFrame);
      fitTimers.forEach(clearTimeout);
      clearTimeout(scrollbackTimeout);
      onDataDisposable.dispose();
      onResizeDisposable.dispose();
      observer.disconnect();
      // Clear handlers before closing to prevent post-unmount state updates
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

  return { status, term: termRef, ws: wsRef, fitTerminal, focusTerminal };
}
