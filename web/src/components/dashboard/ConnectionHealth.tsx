import { Wifi, WifiOff } from 'lucide-react';
import { useMachines } from '../../hooks/useMachines.ts';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import type { Machine } from '../../lib/types.ts';

function statusDot(machine: Machine): { color: string; label: string } {
  if (machine.status !== 'connected') {
    return { color: 'bg-red-500', label: 'Disconnected' };
  }
  if (machine.health) {
    return { color: 'bg-green-500', label: 'Healthy' };
  }
  return { color: 'bg-yellow-500', label: 'No health data' };
}

function AgentCard({ machine }: { machine: Machine }) {
  const dot = statusDot(machine);
  const name = machine.display_name || machine.machine_id;

  return (
    <div className="bg-bg-secondary border border-border-primary rounded-lg p-4 space-y-2">
      <div className="flex items-center gap-2">
        <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${dot.color}`} title={dot.label} />
        <span className="text-sm font-medium text-text-primary truncate">{name}</span>
      </div>

      <div className="text-xs text-text-secondary space-y-1">
        {machine.last_seen_at && (
          <p>
            Last seen: <TimeAgo date={machine.last_seen_at} />
          </p>
        )}
        {machine.health ? (
          <p>
            Sessions: {machine.health.active_sessions} / {machine.health.max_sessions}
          </p>
        ) : (
          <p>Sessions: {machine.status === 'connected' ? 'unknown' : '--'}</p>
        )}
      </div>
    </div>
  );
}

export function ConnectionHealth() {
  const { data: machines, isLoading } = useMachines();

  if (isLoading) {
    return (
      <div className="space-y-3">
        <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2">
          <Wifi size={16} />
          Connection Health
        </h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-bg-secondary border border-border-primary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-tertiary rounded w-2/3 mb-3" />
              <div className="h-3 bg-bg-tertiary rounded w-1/2" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  const list = machines ?? [];

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2">
        <Wifi size={16} />
        Connection Health
      </h3>
      {list.length === 0 ? (
        <div className="bg-bg-secondary border border-border-primary rounded-lg p-6 text-center">
          <WifiOff size={24} className="mx-auto text-text-secondary mb-2" />
          <p className="text-sm text-text-secondary">No machines registered.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {list.map((m) => (
            <AgentCard key={m.machine_id} machine={m} />
          ))}
        </div>
      )}
    </div>
  );
}
