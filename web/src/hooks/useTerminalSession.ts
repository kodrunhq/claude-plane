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

    // Initial fit deferred to next frame so the container has layout dimensions.
    const initialFitFrame = requestAnimationFrame(() => {
      fitAddon.fit();
    });

    termRef.current = term;
    fitAddonRef.current = fitAddon;

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
      fitAddon.fit();

      // Safety timeout: if scrollback_end never arrives, transition to live
      scrollbackTimeout = window.setTimeout(() => {
        setStatus((prev) => (prev === 'replaying' ? 'live' : prev));
      }, 10_000);
    };

    ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else {
        try {
          const msg = JSON.parse(event.data as string) as { type: string };
          if (msg.type === 'scrollback_end') {
            clearTimeout(scrollbackTimeout);
            setStatus('live');
            // Force a SIGWINCH on the PTY to make the CLI redraw its screen.
            // The PTY was created at an estimated size that may match xterm.js,
            // in which case fit() is a no-op (no resize sent, no SIGWINCH).
            // To guarantee a redraw: resize to 1 col smaller, then back.
            // This two-step resize always triggers SIGWINCH regardless of
            // whether the initial PTY size matched.
            term.reset();
            const currentCols = term.cols;
            const currentRows = term.rows;
            if (ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({ type: 'resize', cols: Math.max(1, currentCols - 1), rows: currentRows }));
              // Restore correct size after a tick
              setTimeout(() => {
                fitAddon.fit();
              }, 50);
            } else {
              fitAddon.fit();
            }
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

    // Keystrokes -> server (binary frames)
    const onDataDisposable = term.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
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
      }, 50);
    });
    observer.observe(containerEl);

    return () => {
      if (resizeTimer) clearTimeout(resizeTimer);
      cancelAnimationFrame(initialFitFrame);
      clearTimeout(scrollbackTimeout);
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

  return { status, term: termRef, ws: wsRef, fitTerminal, focusTerminal };
}
