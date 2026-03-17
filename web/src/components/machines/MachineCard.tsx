import { useState, useRef, useEffect } from 'react';
import { Check, Cpu, MemoryStick, Pencil, Terminal, X } from 'lucide-react';
import { StatusBadge } from '../shared/StatusBadge.tsx';
import { TimeAgo } from '../shared/TimeAgo.tsx';
import { useUpdateMachine } from '../../hooks/useMachines.ts';
import type { Machine } from '../../lib/types.ts';

interface MachineCardProps {
  machine: Machine;
  onCreateSession: (machineId: string) => void;
}

function formatMemory(totalMB: number, usedMB: number): string {
  const fmt = (mb: number) => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`;
  return `${fmt(usedMB)} / ${fmt(totalMB)}`;
}

function memoryPercent(totalMB: number, usedMB: number): number {
  if (totalMB <= 0) return 0;
  return Math.round((usedMB / totalMB) * 100);
}

export function MachineCard({ machine, onCreateSession }: MachineCardProps) {
  const isConnected = machine.status === 'connected';
  const health = machine.health;
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState(machine.display_name || '');
  const inputRef = useRef<HTMLInputElement>(null);
  const updateMachine = useUpdateMachine();

  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [isEditing]);

  const handleSave = () => {
    const trimmed = editValue.trim();
    if (trimmed === '' || trimmed.length > 255) return;
    updateMachine.mutate(
      { id: machine.machine_id, params: { display_name: trimmed } },
      { onSuccess: () => setIsEditing(false) },
    );
  };

  const handleCancel = () => {
    setEditValue(machine.display_name || '');
    setIsEditing(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSave();
    if (e.key === 'Escape') handleCancel();
  };

  return (
    <div
      className="gradient-border-card p-4"
      style={{ '--glow-color': isConnected ? '#22c55e' : '#64748b' } as React.CSSProperties}
    >
      <div className="flex items-center justify-between mb-3">
        <StatusBadge status={machine.status} size="sm" />
        <TimeAgo date={machine.last_seen_at} className="text-xs text-text-secondary" />
      </div>

      <div className="mb-3">
        {isEditing ? (
          <div className="flex items-center gap-1">
            <input
              ref={inputRef}
              type="text"
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              onKeyDown={handleKeyDown}
              maxLength={255}
              className="flex-1 min-w-0 px-1.5 py-0.5 text-sm rounded border border-border-primary bg-bg-secondary text-text-primary focus:outline-none focus:border-accent-primary"
              disabled={updateMachine.isPending}
            />
            <button
              onClick={handleSave}
              disabled={updateMachine.isPending}
              className="p-0.5 text-accent-green hover:text-accent-green/80 disabled:opacity-50"
              title="Save"
            >
              <Check size={14} />
            </button>
            <button
              onClick={handleCancel}
              disabled={updateMachine.isPending}
              className="p-0.5 text-text-secondary hover:text-text-primary disabled:opacity-50"
              title="Cancel"
            >
              <X size={14} />
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-1 group">
            <p className="text-sm text-text-primary font-medium truncate">
              {machine.display_name || machine.machine_id}
            </p>
            <button
              onClick={() => {
                setEditValue(machine.display_name || '');
                setIsEditing(true);
              }}
              className="p-0.5 text-text-secondary opacity-0 group-hover:opacity-100 hover:text-text-primary transition-opacity"
              title="Rename machine"
            >
              <Pencil size={12} />
            </button>
          </div>
        )}
        <span className="font-mono text-xs truncate max-w-[140px] opacity-60 text-text-secondary" title={machine.machine_id}>
          {machine.machine_id.slice(0, 12)}
        </span>
      </div>

      {/* Resource Metrics */}
      {health ? (
        <div className="space-y-2 mb-3">
          <div className="flex items-center gap-2 text-xs text-text-secondary">
            <Terminal size={13} className="text-accent-primary shrink-0" />
            <span>{health.active_sessions} active session{health.active_sessions !== 1 ? 's' : ''}</span>
          </div>
          <div className="flex items-center gap-2 text-xs text-text-secondary">
            <Cpu size={13} className="text-accent-primary shrink-0" />
            <span>{health.cpu_cores} cores</span>
          </div>
          <div className="flex items-center gap-2 text-xs text-text-secondary">
            <MemoryStick size={13} className="text-accent-primary shrink-0" />
            <span>{formatMemory(health.memory_total_mb, health.memory_used_mb)}</span>
            <span className="text-text-secondary/60">({memoryPercent(health.memory_total_mb, health.memory_used_mb)}%)</span>
          </div>
        </div>
      ) : (
        <div className="text-xs text-text-secondary mb-3 opacity-60">
          {isConnected ? 'Awaiting health data...' : 'Offline'}
        </div>
      )}

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
