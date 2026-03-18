import { useEffect, useRef, useState } from 'react';
import type { LogEntry, LogFilter } from '../types/log.ts';

const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000, 30000];

/** Extract only the fields the WebSocket server supports for filtering. */
function buildWsFilter(f: LogFilter) {
  return {
    level: f.level,
    component: f.component,
    source: f.source,
    machine_id: f.machine_id,
  };
}

export function useLogStream(
  filter: LogFilter,
  enabled: boolean = false,
): { entries: LogEntry[]; connected: boolean } {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const retriesRef = useRef(0);
  const mountedRef = useRef(true);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const filterRef = useRef(filter);
  const nextIdRef = useRef(1);

  // Keep filterRef in sync and send filter updates
  useEffect(() => {
    filterRef.current = filter;
    setEntries([]); // Clear stale entries on filter change
    nextIdRef.current = 1; // Reset client-side ID counter
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({
        type: 'update_filter',
        ...buildWsFilter(filter),
      }));
    }
  }, [filter]);

  useEffect(() => {
    mountedRef.current = true;

    if (!enabled) return;

    function connect() {
      if (!mountedRef.current) return;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws/logs`);
      wsRef.current = ws;

      ws.onopen = () => {
        if (!mountedRef.current) {
          ws.close();
          return;
        }
        setConnected(true);
        retriesRef.current = 0;
        // Send initial filter (only server-supported fields)
        ws.send(JSON.stringify({
          type: 'subscribe',
          ...buildWsFilter(filterRef.current),
        }));
      };

      ws.onmessage = (event: MessageEvent) => {
        try {
          const msg = JSON.parse(event.data as string) as { type: string; entry?: LogEntry };
          if (msg.type === 'log' && msg.entry) {
            const entry = { ...msg.entry, id: nextIdRef.current++ };
            setEntries(prev => [entry, ...prev].slice(0, 1000));
          }
        } catch {
          // ignore parse errors
        }
      };

      ws.onerror = () => {
        // onclose will fire after this
      };

      ws.onclose = () => {
        if (!mountedRef.current) return;
        setConnected(false);
        wsRef.current = null;

        // Reconnect with exponential backoff
        const delay = RECONNECT_DELAYS[Math.min(retriesRef.current, RECONNECT_DELAYS.length - 1)];
        retriesRef.current += 1;
        reconnectTimerRef.current = setTimeout(connect, delay);
      };
    }

    connect();

    return () => {
      mountedRef.current = false;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (wsRef.current) {
        const ws = wsRef.current;
        ws.onopen = null;
        ws.onmessage = null;
        ws.onerror = null;
        ws.onclose = null;
        ws.close();
        wsRef.current = null;
      }
    };
  }, [enabled]);

  return { entries, connected };
}
