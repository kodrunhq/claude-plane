import { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import type { Event } from '../../types/event.ts';

interface EventsTableProps {
  events: Event[];
}

function payloadPreview(payload: Record<string, unknown>): string {
  const json = JSON.stringify(payload);
  return json.length > 80 ? json.slice(0, 80) + '...' : json;
}

function formatTimestamp(ts: string): string {
  try {
    return new Date(ts).toLocaleString(undefined, {
      year: 'numeric',
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return ts;
  }
}

interface ExpandedRowProps {
  payload: Record<string, unknown>;
}

function ExpandedPayload({ payload }: ExpandedRowProps) {
  return (
    <tr className="bg-bg-primary/60">
      <td colSpan={5} className="px-6 py-4">
        <pre className="text-xs text-text-secondary font-mono whitespace-pre-wrap break-all bg-bg-secondary border border-gray-700 rounded-md p-4 max-h-64 overflow-auto">
          {JSON.stringify(payload, null, 2)}
        </pre>
      </td>
    </tr>
  );
}

interface EventRowProps {
  event: Event;
  expanded: boolean;
  onToggle: () => void;
}

function EventRow({ event, expanded, onToggle }: EventRowProps) {
  const hasPayload = Object.keys(event.payload).length > 0;

  return (
    <>
      <tr
        className="border-t border-gray-700 hover:bg-bg-tertiary/30 transition-colors"
        onClick={hasPayload ? onToggle : undefined}
        style={{ cursor: hasPayload ? 'pointer' : 'default' }}
      >
        <td className="px-4 py-3 w-8">
          {hasPayload ? (
            <button
              onClick={(e) => { e.stopPropagation(); onToggle(); }}
              className="text-text-secondary hover:text-text-primary transition-colors"
              aria-label={expanded ? 'Collapse payload' : 'Expand payload'}
            >
              {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            </button>
          ) : (
            <span className="w-3.5 inline-block" />
          )}
        </td>
        <td className="px-4 py-3">
          <span className="font-mono text-xs px-2 py-0.5 rounded bg-accent-primary/10 text-accent-primary border border-accent-primary/20">
            {event.event_type}
          </span>
        </td>
        <td className="px-4 py-3 text-sm text-text-secondary font-mono whitespace-nowrap">
          {formatTimestamp(event.timestamp)}
        </td>
        <td className="px-4 py-3 text-sm text-text-secondary">
          {event.source || '—'}
        </td>
        <td className="px-4 py-3 text-xs text-text-secondary font-mono max-w-xs truncate">
          {hasPayload ? payloadPreview(event.payload) : '{}'}
        </td>
      </tr>
      {expanded && hasPayload && <ExpandedPayload payload={event.payload} />}
    </>
  );
}

export function EventsTable({ events }: EventsTableProps) {
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  function toggleRow(eventId: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(eventId)) {
        next.delete(eventId);
      } else {
        next.add(eventId);
      }
      return next;
    });
  }

  return (
    <div className="overflow-hidden rounded-lg border border-gray-700">
      <table className="w-full border-collapse">
        <thead>
          <tr className="bg-bg-secondary">
            <th className="px-4 py-3 w-8" />
            <th className="px-4 py-3 text-left text-xs font-semibold text-text-secondary uppercase tracking-wider">
              Event Type
            </th>
            <th className="px-4 py-3 text-left text-xs font-semibold text-text-secondary uppercase tracking-wider">
              Timestamp
            </th>
            <th className="px-4 py-3 text-left text-xs font-semibold text-text-secondary uppercase tracking-wider">
              Source
            </th>
            <th className="px-4 py-3 text-left text-xs font-semibold text-text-secondary uppercase tracking-wider">
              Payload
            </th>
          </tr>
        </thead>
        <tbody>
          {events.map((event) => (
            <EventRow
              key={event.event_id}
              event={event}
              expanded={expandedIds.has(event.event_id)}
              onToggle={() => toggleRow(event.event_id)}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
