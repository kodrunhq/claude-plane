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
) {
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [status, setStatus] = useState<TerminalStatus>('connecting');

  // eslint-disable-next-line react-hooks/exhaustive-deps -- containerRef is a stable ref, intentionally excluded
  useEffect(() => {
    if (!containerRef.current || !sessionId) return;

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
    try {
      term.loadAddon(new WebglAddon());
    } catch {
      // Falls back to canvas/DOM renderer silently
    }

    term.open(containerRef.current);
    fitAddon.fit();

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // 2. WebSocket connection (first-message auth)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/ws/terminal/${sessionId}`,
    );
    ws.binaryType = 'arraybuffer';

    // eslint-disable-next-line react-hooks/set-state-in-effect -- initializing status for new WS connection
    setStatus('connecting');

    ws.onopen = () => {
      // Cookie-based auth: the session_token cookie is sent automatically
      // on the WebSocket upgrade request, so no first-message auth is needed.
      setStatus('replaying');

      // Send actual terminal dimensions to resize the remote PTY from 80x24
      // to match the browser viewport.
      fitAddon.fit();
      const { cols, rows } = term;
      ws.send(JSON.stringify({ type: 'resize', cols, rows }));
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

    // 5. Container resize -> fit terminal
    const observer = new ResizeObserver(() => {
      fitAddon.fit();
    });
    observer.observe(containerRef.current);

    // Cleanup
    return () => {
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
  }, [sessionId]);

  const fitTerminal = useCallback(() => {
    fitAddonRef.current?.fit();
  }, []);

  return { status, term: termRef, ws: wsRef, fitTerminal };
}
