import { Cpu } from 'lucide-react';
import { useMachines } from '../../hooks/useMachines.ts';
import type { Machine } from '../../lib/types.ts';

function ResourceBar({ label, percent, color }: { label: string; percent: number; color: string }) {
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs">
        <span className="text-text-secondary">{label}</span>
        <span className="text-text-primary font-mono tabular-nums">{percent}%</span>
      </div>
      <div className="h-2 bg-bg-tertiary rounded-full overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-500 ${color}`}
          style={{ width: `${Math.min(percent, 100)}%` }}
        />
      </div>
    </div>
  );
}

function barColor(percent: number): string {
  if (percent >= 90) return 'bg-red-500';
  if (percent >= 70) return 'bg-yellow-500';
  return 'bg-green-500';
}

function AgentResourceCard({ machine }: { machine: Machine }) {
  const name = machine.display_name || machine.machine_id;
  const health = machine.health;

  if (!health) {
    return (
      <div className="bg-bg-secondary border border-border-primary rounded-lg p-4">
        <p className="text-sm font-medium text-text-primary truncate mb-2">{name}</p>
        <p className="text-xs text-text-secondary">No resource data available.</p>
      </div>
    );
  }

  const memPercent =
    health.memory_total_mb > 0
      ? Math.round((health.memory_used_mb / health.memory_total_mb) * 100)
      : 0;
  // CPU cores is informational; we don't have a usage percentage from health,
  // so we derive a "session load" percentage instead.
  const sessionPercent =
    health.max_sessions > 0
      ? Math.round((health.active_sessions / health.max_sessions) * 100)
      : 0;

  return (
    <div className="bg-bg-secondary border border-border-primary rounded-lg p-4 space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium text-text-primary truncate">{name}</p>
        <span className="text-xs text-text-secondary">{health.cpu_cores} cores</span>
      </div>
      <ResourceBar label="Memory" percent={memPercent} color={barColor(memPercent)} />
      <ResourceBar label="Session Load" percent={sessionPercent} color={barColor(sessionPercent)} />
    </div>
  );
}

export function AgentResources() {
  const { data: machines, isLoading } = useMachines();

  const connected = (machines ?? []).filter((m) => m.status === 'connected');

  if (isLoading) {
    return (
      <div className="space-y-3">
        <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2">
          <Cpu size={16} />
          Agent Resources
        </h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {Array.from({ length: 2 }, (_, i) => (
            <div key={i} className="bg-bg-secondary border border-border-primary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-tertiary rounded w-1/2 mb-3" />
              <div className="h-2 bg-bg-tertiary rounded-full w-full mb-2" />
              <div className="h-2 bg-bg-tertiary rounded-full w-3/4" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2">
        <Cpu size={16} />
        Agent Resources
      </h3>
      {connected.length === 0 ? (
        <div className="bg-bg-secondary border border-border-primary rounded-lg p-6 text-center">
          <p className="text-sm text-text-secondary">No connected agents.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {connected.map((m) => (
            <AgentResourceCard key={m.machine_id} machine={m} />
          ))}
        </div>
      )}
    </div>
  );
}
