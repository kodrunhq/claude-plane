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
  options?: { useWebGL?: boolean },
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

    // 1. Create xterm.js instance
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'JetBrains Mono, Menlo, Monaco, Consolas, monospace',
      fontSize: 14,
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

    // Defer initial fit to next frame so the browser has completed layout
    // and the container has its final dimensions. Without this, the first
    // render can have incorrect sizing (fixed by any subsequent resize).
    const initialFitFrame = requestAnimationFrame(() => {
      fitAddon.fit();
    });

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
      setStatus('replaying');

      // Fit the terminal to the container — this triggers term.onResize which
      // sends the resize control message to the server automatically.
      fitAddon.fit();

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
  }, [sessionId, containerEl]);

  const fitTerminal = useCallback(() => {
    fitAddonRef.current?.fit();
  }, []);

  return { status, term: termRef, ws: wsRef, fitTerminal };
}
