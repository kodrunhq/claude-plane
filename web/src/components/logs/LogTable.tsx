import { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import type { LogEntry } from '../../types/log.ts';

const LEVEL_STYLES: Record<string, string> = {
  DEBUG: 'bg-gray-500/20 text-gray-400',
  INFO: 'bg-blue-500/20 text-blue-400',
  WARN: 'bg-amber-500/20 text-amber-400',
  ERROR: 'bg-red-500/20 text-red-400',
};

function formatRelativeTime(timestamp: string): string {
  const diff = Date.now() - new Date(timestamp).getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function LevelBadge({ level }: { level: string }) {
  const style = LEVEL_STYLES[level] ?? LEVEL_STYLES.INFO;
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${style}`}>
      {level}
    </span>
  );
}

function ExpandedRow({ entry }: { entry: LogEntry }) {
  const metadata = entry.metadata ? tryParseJSON(entry.metadata) : null;

  return (
    <tr className="bg-bg-tertiary/50">
      <td colSpan={6} className="px-6 py-3">
        <div className="space-y-2 text-xs">
          {entry.session_id && (
            <div>
              <span className="text-text-secondary">Session ID: </span>
              <span className="text-text-primary font-mono">{entry.session_id}</span>
            </div>
          )}
          {entry.error && (
            <div>
              <span className="text-text-secondary">Error: </span>
              <span className="text-red-400 font-mono">{entry.error}</span>
            </div>
          )}
          {metadata && (
            <div>
              <span className="text-text-secondary">Metadata:</span>
              <pre className="mt-1 p-2 rounded bg-bg-primary text-text-primary font-mono text-xs overflow-x-auto whitespace-pre-wrap">
                {JSON.stringify(metadata, null, 2)}
              </pre>
            </div>
          )}
          <div className="text-text-secondary">
            Full timestamp: {new Date(entry.timestamp).toLocaleString()}
          </div>
        </div>
      </td>
    </tr>
  );
}

function tryParseJSON(str: string): unknown {
  try {
    return JSON.parse(str);
  } catch {
    return str;
  }
}

interface LogTableProps {
  entries: LogEntry[];
  loading: boolean;
}

export function LogTable({ entries, loading }: LogTableProps) {
  const [expandedId, setExpandedId] = useState<number | null>(null);

  if (loading) {
    return null; // Parent handles skeleton
  }

  if (entries.length === 0) {
    return null; // Parent handles empty state
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border-primary">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="bg-bg-secondary text-text-secondary text-xs uppercase tracking-wider">
            <th className="w-8 px-2 py-3" />
            <th className="px-4 py-3 text-left">Time</th>
            <th className="px-4 py-3 text-left">Level</th>
            <th className="px-4 py-3 text-left">Source</th>
            <th className="px-4 py-3 text-left">Component</th>
            <th className="px-4 py-3 text-left">Message</th>
            <th className="px-4 py-3 text-left">Machine</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => {
            const isExpanded = expandedId === entry.id;
            const hasDetails = Boolean(entry.metadata || entry.session_id || entry.error);

            return (
              <LogRow
                key={entry.id}
                entry={entry}
                isExpanded={isExpanded}
                hasDetails={hasDetails}
                onToggle={() => setExpandedId(isExpanded ? null : entry.id)}
              />
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function LogRow({
  entry,
  isExpanded,
  hasDetails,
  onToggle,
}: {
  entry: LogEntry;
  isExpanded: boolean;
  hasDetails: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <tr
        onClick={hasDetails ? onToggle : undefined}
        className={`border-t border-border-primary transition-colors ${
          hasDetails ? 'cursor-pointer hover:bg-bg-tertiary/40' : ''
        } ${isExpanded ? 'bg-bg-tertiary/30' : ''}`}
      >
        <td className="px-2 py-2.5 text-center text-text-secondary">
          {hasDetails &&
            (isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />)}
        </td>
        <td className="px-4 py-2.5 text-text-secondary whitespace-nowrap font-mono text-xs">
          {formatRelativeTime(entry.timestamp)}
        </td>
        <td className="px-4 py-2.5">
          <LevelBadge level={entry.level} />
        </td>
        <td className="px-4 py-2.5 text-text-secondary capitalize">{entry.source}</td>
        <td className="px-4 py-2.5 text-text-secondary font-mono text-xs">{entry.component}</td>
        <td className="px-4 py-2.5 text-text-primary truncate max-w-md">{entry.message}</td>
        <td className="px-4 py-2.5 text-text-secondary font-mono text-xs">
          {entry.machine_id ? entry.machine_id.slice(0, 8) : '\u2014'}
        </td>
      </tr>
      {isExpanded && <ExpandedRow entry={entry} />}
    </>
  );
}
