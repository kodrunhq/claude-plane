import { useEffect, useMemo, useRef, useState } from 'react';
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

/** Stable string key for the current filter — used to reset entries on change. */
function filterKey(f: LogFilter): string {
  return JSON.stringify(buildWsFilter(f));
}

export function useLogStream(
  filter: LogFilter,
  enabled: boolean = false,
): { entries: LogEntry[]; connected: boolean } {
  // Reset entries whenever the filter changes by keying on filterKey.
  // This avoids calling setState synchronously inside an effect.
  const fKey = useMemo(() => filterKey(filter), [filter]);
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const retriesRef = useRef(0);
  const mountedRef = useRef(true);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const filterRef = useRef(filter);
  const nextIdRef = useRef(1);
  const prevFilterKeyRef = useRef(fKey);

  // Keep filterRef in sync and send filter updates to the server.
  // Entries are cleared via the prevFilterKeyRef check in onmessage,
  // not via a synchronous setState in this effect.
  useEffect(() => {
    filterRef.current = filter;
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
            const currentKey = filterKey(filterRef.current);
            const entry = { ...msg.entry, id: nextIdRef.current++ };
            setEntries(prev => {
              // If filter changed since last message, clear old entries
              if (prevFilterKeyRef.current !== currentKey) {
                prevFilterKeyRef.current = currentKey;
                nextIdRef.current = 2; // reset after the current entry
                return [entry];
              }
              return [entry, ...prev].slice(0, 1000);
            });
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
