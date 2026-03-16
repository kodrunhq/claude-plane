import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useRunStore } from '../stores/runs.ts';
import {
  SESSION_STARTED,
  SESSION_EXITED,
  SESSION_TERMINATED,
  MACHINE_CONNECTED,
  MACHINE_DISCONNECTED,
  RUN_CREATED,
  RUN_STARTED,
  RUN_COMPLETED,
  RUN_FAILED,
  RUN_CANCELLED,
  RUN_STEP_COMPLETED,
  RUN_STEP_FAILED,
  TEMPLATE_CREATED,
  TEMPLATE_UPDATED,
  TEMPLATE_DELETED,
} from '../constants/eventTypes.ts';

/** Wire format from WSFanout (internal/server/event/ws_fanout.go). */
interface WsEventMsg {
  type: 'event';
  event_id: string;
  event_type: string;
  timestamp: string;
  source: string;
  payload: Record<string, unknown>;
}

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
          const msg = JSON.parse(event.data as string) as WsEventMsg;
          if (msg.type !== 'event') return;

          switch (msg.event_type) {
            case SESSION_STARTED:
            case SESSION_EXITED:
            case SESSION_TERMINATED:
              queryClient.invalidateQueries({ queryKey: ['sessions'] });
              break;
            case MACHINE_CONNECTED:
            case MACHINE_DISCONNECTED:
              queryClient.invalidateQueries({ queryKey: ['machines'] });
              break;
            case RUN_CREATED:
            case RUN_STARTED:
            case RUN_COMPLETED:
            case RUN_FAILED:
            case RUN_CANCELLED:
              queryClient.invalidateQueries({ queryKey: ['runs'] });
              break;
            case RUN_STEP_COMPLETED:
            case RUN_STEP_FAILED: {
              const p = msg.payload as { run_id?: string; step_id?: string; status?: string; session_id?: string };
              if (p.run_id && p.step_id && p.status) {
                useRunStore.getState().updateTaskStatus(p.run_id, p.step_id, p.status, p.session_id);
              }
              queryClient.invalidateQueries({ queryKey: ['runs'] });
              break;
            }
            case TEMPLATE_CREATED:
            case TEMPLATE_UPDATED:
            case TEMPLATE_DELETED:
              queryClient.invalidateQueries({ queryKey: ['templates'] });
              break;
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
