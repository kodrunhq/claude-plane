import { useEffect, useRef } from 'react';
import { toast } from 'sonner';
import {
  MACHINE_DISCONNECTED,
  MACHINE_STALE,
  SESSION_DISPATCH_FAILED,
} from '../constants/eventTypes.ts';

interface SystemEvent {
  event_type: string;
  event_id?: string;
  payload: Record<string, unknown>;
}

/**
 * Listens for critical events broadcast by useEventStream via the
 * 'claude-plane-event' CustomEvent and shows sonner toast notifications.
 *
 * Call once in a top-level component (e.g. AppShell).
 */
export function useSystemAlerts() {
  const handledRef = useRef(new Set<string>());

  useEffect(() => {
    function handleEvent(e: Event) {
      const detail = (e as CustomEvent<SystemEvent>).detail;
      if (!detail?.event_type) return;

      // Deduplicate by event_id when available
      const id = detail.event_id;
      if (id) {
        if (handledRef.current.has(id)) return;
        handledRef.current.add(id);
        // Cap set size to prevent memory leak
        if (handledRef.current.size > 1000) {
          handledRef.current.clear();
        }
      }

      const payload = detail.payload ?? {};

      switch (detail.event_type) {
        case MACHINE_DISCONNECTED:
          toast.error(
            `Agent ${String(payload.machine_id ?? 'unknown')} disconnected`,
            {
              description: 'The agent connection was lost.',
              action: {
                label: 'View Logs',
                onClick: () =>
                  window.location.assign(
                    '/logs?source=server&component=connmgr',
                  ),
              },
            },
          );
          break;

        case MACHINE_STALE:
          toast.warning(
            `Agent ${String(payload.machine_id ?? 'unknown')} stale`,
            {
              description: 'Dead transport detected and cleaned up.',
            },
          );
          break;

        case SESSION_DISPATCH_FAILED:
          toast.error('Session dispatch failed', {
            description: String(payload.error ?? 'Unknown error'),
            action: {
              label: 'View Logs',
              onClick: () =>
                window.location.assign(
                  '/logs?level=ERROR&component=session',
                ),
            },
          });
          break;
      }
    }

    window.addEventListener('claude-plane-event', handleEvent);
    return () => window.removeEventListener('claude-plane-event', handleEvent);
  }, []);
}
