import { useCallback, useEffect, useRef, useState } from 'react';
import {
  MACHINE_DISCONNECTED,
  MACHINE_STALE,
  SESSION_DISPATCH_FAILED,
  RUN_FAILED,
} from '../../constants/eventTypes.ts';

type Severity = 'error' | 'warning';

interface AlertEntry {
  id: string;
  severity: Severity;
  message: string;
  timestamp: number;
}

const CRITICAL_EVENTS: Record<string, Severity> = {
  [MACHINE_DISCONNECTED]: 'error',
  [MACHINE_STALE]: 'warning',
  [SESSION_DISPATCH_FAILED]: 'error',
  [RUN_FAILED]: 'warning',
};

const MAX_ENTRIES = 5;
const STALE_MS = 5 * 60 * 1000; // 5 minutes

function formatMessage(eventType: string, payload: Record<string, unknown>): string {
  switch (eventType) {
    case MACHINE_DISCONNECTED:
      return `Agent ${String(payload.machine_id ?? 'unknown')} disconnected`;
    case MACHINE_STALE:
      return `Agent ${String(payload.machine_id ?? 'unknown')} stale`;
    case SESSION_DISPATCH_FAILED:
      return `Session dispatch failed: ${String(payload.error ?? 'unknown')}`;
    case RUN_FAILED:
      return `Run ${String(payload.run_id ?? 'unknown')} failed`;
    default:
      return eventType;
  }
}

export function HealthIndicator() {
  const [alerts, setAlerts] = useState<readonly AlertEntry[]>([]);
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const counterRef = useRef(0);

  // Prune stale entries periodically
  useEffect(() => {
    const interval = setInterval(() => {
      const cutoff = Date.now() - STALE_MS;
      setAlerts((prev) => {
        const filtered = prev.filter((a) => a.timestamp > cutoff);
        return filtered.length === prev.length ? prev : filtered;
      });
    }, 30_000);
    return () => clearInterval(interval);
  }, []);

  // Listen for events
  useEffect(() => {
    function handleEvent(e: Event) {
      const detail = (e as CustomEvent<{ event_type: string; payload: Record<string, unknown> }>).detail;
      if (!detail?.event_type) return;

      const severity = CRITICAL_EVENTS[detail.event_type];
      if (!severity) return;

      counterRef.current += 1;
      const entry: AlertEntry = {
        id: String(counterRef.current),
        severity,
        message: formatMessage(detail.event_type, detail.payload ?? {}),
        timestamp: Date.now(),
      };

      setAlerts((prev) => [entry, ...prev].slice(0, MAX_ENTRIES));
    }

    window.addEventListener('claude-plane-event', handleEvent);
    return () => window.removeEventListener('claude-plane-event', handleEvent);
  }, []);

  // Close dropdown on outside click
  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open]);

  const toggleOpen = useCallback(() => setOpen((v) => !v), []);

  // Determine dot color based on active alerts.
  // The periodic prune (above) already removes entries older than STALE_MS,
  // so `alerts` always reflects the current window.
  const hasError = alerts.some((a) => a.severity === 'error');
  const hasWarning = alerts.some((a) => a.severity === 'warning');

  let dotColor = 'bg-accent-green';
  let dotLabel = 'System healthy';
  if (hasError) {
    dotColor = 'bg-red-500';
    dotLabel = 'System errors present';
  } else if (hasWarning) {
    dotColor = 'bg-yellow-500';
    dotLabel = 'System warnings present';
  }

  return (
    <div className="relative" ref={containerRef}>
      <button
        onClick={toggleOpen}
        className="p-2 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
        aria-label={dotLabel}
        title={dotLabel}
      >
        <span className={`inline-block w-2.5 h-2.5 rounded-full ${dotColor} ${hasError || hasWarning ? '' : 'animate-pulse'}`} />
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 w-72 bg-bg-secondary border border-border-primary rounded-lg shadow-lg z-50 overflow-hidden">
          <div className="px-3 py-2 border-b border-border-primary">
            <span className="text-xs font-semibold text-text-secondary uppercase tracking-wider">
              System Health
            </span>
          </div>
          {alerts.length === 0 ? (
            <div className="px-3 py-4 text-center text-sm text-text-secondary">
              No issues in the last 5 minutes
            </div>
          ) : (
            <ul className="max-h-60 overflow-y-auto">
              {alerts.map((a) => (
                <li
                  key={a.id}
                  className="px-3 py-2 border-b border-border-primary last:border-b-0 flex items-start gap-2"
                >
                  <span
                    className={`mt-1 inline-block w-2 h-2 rounded-full shrink-0 ${
                      a.severity === 'error' ? 'bg-red-500' : 'bg-yellow-500'
                    }`}
                  />
                  <div className="min-w-0">
                    <p className="text-sm text-text-primary truncate">{a.message}</p>
                    <p className="text-xs text-text-secondary">
                      {new Date(a.timestamp).toLocaleTimeString()}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
