import { StatusBadge } from '../shared/StatusBadge.tsx';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import type { Machine } from '../../lib/types.ts';

interface MachineCardProps {
  machine: Machine;
  onCreateSession: (machineId: string) => void;
}

export function MachineCard({ machine, onCreateSession }: MachineCardProps) {
  const isConnected = machine.status === 'connected';

  return (
    <div
      className="gradient-border-card p-4"
      style={{ '--glow-color': isConnected ? '#22c55e' : '#64748b' } as React.CSSProperties}
    >
      <div className="flex items-center justify-between mb-3">
        <StatusBadge status={machine.status} size="sm" />
        <span className="text-xs text-text-secondary">
          max {machine.max_sessions} session{machine.max_sessions !== 1 ? 's' : ''}
        </span>
      </div>

      <div className="mb-2">
        <p className="text-sm text-text-primary font-medium truncate">
          {machine.display_name || machine.machine_id}
        </p>
      </div>

      <div className="flex items-center justify-between text-xs text-text-secondary mb-3">
        <span className="font-mono truncate max-w-[140px] opacity-60" title={machine.machine_id}>
          {machine.machine_id.slice(0, 12)}
        </span>
        <TimeAgo date={machine.last_seen_at} className="text-text-secondary" />
      </div>

      <button
        disabled={!isConnected}
        onClick={() => onCreateSession(machine.machine_id)}
        className="w-full px-3 py-1.5 text-xs rounded-md font-medium bg-accent-green/10 text-accent-green hover:bg-accent-green/20 transition-all hover:shadow-[0_0_12px_rgba(34,197,94,0.15)] disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:shadow-none"
      >
        New Session
      </button>
    </div>
  );
}
