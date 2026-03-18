import { Link } from 'react-router';
import { AlertTriangle, CheckCircle } from 'lucide-react';
import { useLogs } from '../../hooks/useLogs.ts';
import { TimeAgo } from '../shared/TimeAgo.tsx';

export function RecentErrors() {
  const { data, isLoading } = useLogs({ level: 'ERROR', limit: 10 });
  const logs = data?.logs ?? [];

  return (
    <div className="bg-bg-secondary border border-border-primary rounded-lg">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border-primary">
        <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2">
          <AlertTriangle size={16} />
          Recent Errors
        </h3>
        <Link
          to="/logs?level=ERROR"
          className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
        >
          View all
        </Link>
      </div>

      {isLoading ? (
        <div className="p-4 space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="animate-pulse flex gap-3">
              <div className="h-3 bg-bg-tertiary rounded w-16" />
              <div className="h-3 bg-bg-tertiary rounded flex-1" />
            </div>
          ))}
        </div>
      ) : logs.length === 0 ? (
        <div className="p-6 text-center">
          <CheckCircle size={24} className="mx-auto text-green-500 mb-2" />
          <p className="text-sm text-text-secondary">No recent errors</p>
        </div>
      ) : (
        <div className="divide-y divide-border-primary">
          {logs.map((log) => (
            <div key={log.id} className="px-4 py-2.5 flex items-start gap-3 text-xs">
              <span className="text-text-secondary whitespace-nowrap shrink-0">
                <TimeAgo date={log.timestamp} />
              </span>
              <span className="inline-flex items-center px-1.5 py-0.5 rounded bg-bg-tertiary text-text-secondary font-mono shrink-0">
                {log.component}
              </span>
              <span className="text-text-primary truncate" title={log.message}>
                {log.message}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
