import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { EventMessage } from '../lib/types.ts';
import { useRunStore } from '../stores/runs.ts';

const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000];

export function useEventStream() {
  const queryClient = useQueryClient();
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const attemptRef = useRef(0);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;

    function connect() {
      if (!mountedRef.current) return;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const ws = new WebSocket(
        `${protocol}//${window.location.host}/ws/events`,
      );

      ws.onopen = () => {
        if (!mountedRef.current) {
          ws.close();
          return;
        }
        // Cookie-based auth: the session_token cookie is sent automatically
        // on the WebSocket upgrade request, so no first-message auth is needed.
        attemptRef.current = 0;
        setConnected(true);
      };

      ws.onmessage = (event: MessageEvent) => {
        try {
          const msg = JSON.parse(event.data as string) as EventMessage;
          switch (msg.type) {
            case 'session.started':
            case 'session.exited':
            case 'session.terminated':
              queryClient.invalidateQueries({ queryKey: ['sessions'] });
              break;
            case 'machine.status':
            case 'machine.health':
              queryClient.invalidateQueries({ queryKey: ['machines'] });
              break;
            case 'run.step.status': {
              const p = msg.payload as { runId?: string; stepId?: string; status?: string; sessionId?: string };
              if (p.runId && p.stepId && p.status) {
                useRunStore.getState().updateTaskStatus(p.runId, p.stepId, p.status, p.sessionId);
              }
              queryClient.invalidateQueries({ queryKey: ['runs'] });
              break;
            }
          }
        } catch {
          // Ignore unparseable messages
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
        const delay = RECONNECT_DELAYS[Math.min(attemptRef.current, RECONNECT_DELAYS.length - 1)];
        attemptRef.current += 1;
        setTimeout(connect, delay);
      };

      wsRef.current = ws;
    }

    connect();

    return () => {
      mountedRef.current = false;
      if (wsRef.current) {
        // Clear handlers before closing to prevent reconnect
        const ws = wsRef.current;
        ws.onopen = null;
        ws.onmessage = null;
        ws.onerror = null;
        ws.onclose = null;
        ws.close();
        wsRef.current = null;
      }
    };
  }, [queryClient]);

  return { connected };
}
