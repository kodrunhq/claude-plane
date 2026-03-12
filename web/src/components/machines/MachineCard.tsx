import { StatusBadge } from '../shared/StatusBadge.tsx';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import type { Machine } from '../../lib/types.ts';

interface MachineCardProps {
  machine: Machine;
  onCreateSession: (machineId: string) => void;
}

export function MachineCard({ machine, onCreateSession }: MachineCardProps) {
  const isOnline = machine.status === 'online';

  return (
    <div className="bg-bg-tertiary rounded-lg p-4 hover:ring-1 ring-accent-primary transition">
      <div className="flex items-center justify-between mb-3">
        <StatusBadge status={machine.status} size="sm" />
        {machine.session_count != null && machine.session_count > 0 && (
          <span className="text-xs bg-accent-primary/10 text-accent-primary px-2 py-0.5 rounded-full">
            {machine.session_count} session{machine.session_count !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      <div className="mb-2">
        <p className="text-sm text-text-primary font-medium truncate">
          {machine.hostname}
        </p>
        <p className="text-xs text-text-secondary mt-0.5">
          {machine.os} / {machine.arch}
        </p>
      </div>

      <div className="flex items-center justify-between text-xs text-text-secondary mb-3">
        <span className="font-mono truncate max-w-[140px]" title={machine.machine_id}>
          {machine.machine_id.slice(0, 12)}
        </span>
        <TimeAgo date={machine.last_seen_at} className="text-text-secondary" />
      </div>

      <button
        disabled={!isOnline}
        onClick={() => onCreateSession(machine.machine_id)}
        className="w-full px-3 py-1.5 text-xs rounded-md bg-accent-primary/10 text-accent-primary hover:bg-accent-primary/20 transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
      >
        New Session
      </button>
    </div>
  );
}
