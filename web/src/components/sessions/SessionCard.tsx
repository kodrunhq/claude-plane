import { StatusBadge } from '../shared/StatusBadge.tsx';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import type { Session } from '../../types/session.ts';

interface SessionCardProps {
  session: Session;
  onAttach: (id: string) => void;
  onTerminate: (id: string) => void;
}

export function SessionCard({ session, onAttach, onTerminate }: SessionCardProps) {
  const isActive = session.status === 'running' || session.status === 'created';

  return (
    <div
      className="gradient-border-card p-4 cursor-pointer"
      style={{ '--glow-color': '#06b6d4' } as React.CSSProperties}
      onClick={() => onAttach(session.session_id)}
    >
      <div className="flex items-center justify-between mb-3">
        <StatusBadge status={session.status} size="sm" />
        <span className="text-xs text-text-secondary font-mono opacity-60" title={session.session_id}>
          {session.session_id.slice(0, 8)}
        </span>
      </div>

      <div className="mb-2">
        <p className="text-sm text-text-primary font-mono truncate">
          {session.command || 'claude'}
        </p>
        {session.working_dir && (
          <p className="text-xs text-text-secondary truncate mt-0.5">
            {session.working_dir}
          </p>
        )}
      </div>

      <div className="flex items-center justify-between text-xs text-text-secondary">
        <span className="font-mono truncate max-w-[120px] opacity-60" title={session.machine_id}>
          {session.machine_id.slice(0, 8)}
        </span>
        <TimeAgo date={session.updated_at} className="text-text-secondary" />
      </div>

      {isActive && (
        <div className="flex gap-2 mt-3 pt-3 border-t border-border-primary">
          <button
            className="flex-1 px-3 py-1.5 text-xs rounded-md font-medium bg-accent-cyan/10 text-accent-cyan hover:bg-accent-cyan/20 transition-all hover:shadow-[0_0_12px_rgba(6,182,212,0.15)]"
            onClick={(e) => {
              e.stopPropagation();
              onAttach(session.session_id);
            }}
          >
            Attach
          </button>
          <button
            className="flex-1 px-3 py-1.5 text-xs rounded-md font-medium bg-status-error/10 text-status-error hover:bg-status-error/20 transition-all hover:shadow-[0_0_12px_rgba(239,68,68,0.15)]"
            onClick={(e) => {
              e.stopPropagation();
              onTerminate(session.session_id);
            }}
          >
            Terminate
          </button>
        </div>
      )}
    </div>
  );
}
