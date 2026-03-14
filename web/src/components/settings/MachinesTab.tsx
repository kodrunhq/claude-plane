import { useState, useCallback } from 'react';
import { Save, ChevronDown, ChevronRight, Server } from 'lucide-react';
import { toast } from 'sonner';
import { useMachines } from '../../hooks/useMachines.ts';
import { KeyValueEditor, envToEntries, entriesToEnv } from './KeyValueEditor.tsx';
import type { UserPreferences, MachineOverride } from '../../types/preferences.ts';
import type { Machine } from '../../lib/types.ts';

interface MachinesTabProps {
  preferences: UserPreferences;
  onSave: (prefs: UserPreferences) => Promise<void>;
  saving: boolean;
}

const MODEL_OPTIONS = [
  { value: '', label: 'Default' },
  { value: 'opus', label: 'Opus' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'haiku', label: 'Haiku' },
] as const;

function emptyOverride(): MachineOverride {
  return {
    working_dir: '',
    model: '',
    env_vars: {},
    max_concurrent_sessions: 0,
  };
}

export function MachinesTab({ preferences, onSave, saving }: MachinesTabProps) {
  const { data: machines, isLoading } = useMachines();
  const [overrides, setOverrides] = useState<Record<string, MachineOverride>>(
    preferences.machine_overrides ?? {},
  );
  const [expandedId, setExpandedId] = useState<string | null>(null);

  function getOverride(machineId: string): MachineOverride {
    return overrides[machineId] ?? emptyOverride();
  }

  function updateOverride(machineId: string, patch: Partial<MachineOverride>) {
    setOverrides((prev) => ({
      ...prev,
      [machineId]: { ...getOverride(machineId), ...patch },
    }));
  }

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    const cleaned: Record<string, MachineOverride> = {};
    for (const [id, override] of Object.entries(overrides)) {
      const hasValues = override.working_dir || override.model ||
        Object.keys(override.env_vars).length > 0 || override.max_concurrent_sessions > 0;
      if (hasValues) {
        cleaned[id] = override;
      }
    }
    try {
      await onSave({
        ...preferences,
        machine_overrides: Object.keys(cleaned).length > 0 ? cleaned : undefined,
      });
      toast.success('Machine overrides saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  }, [preferences, overrides, onSave]);

  if (isLoading) {
    return (
      <div className="space-y-3">
        {Array.from({ length: 3 }, (_, i) => (
          <div key={i} className="bg-bg-tertiary rounded-lg p-4 animate-pulse">
            <div className="h-4 bg-bg-secondary rounded w-1/3" />
          </div>
        ))}
      </div>
    );
  }

  if (!machines || machines.length === 0) {
    return (
      <div className="bg-bg-secondary rounded-lg border border-border-primary p-6 text-center">
        <p className="text-sm text-text-secondary">No machines registered yet.</p>
      </div>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {machines.map((machine: Machine) => {
        const isExpanded = expandedId === machine.machine_id;
        const override = getOverride(machine.machine_id);
        return (
          <MachineOverrideCard
            key={machine.machine_id}
            machine={machine}
            override={override}
            expanded={isExpanded}
            onToggle={() => setExpandedId(isExpanded ? null : machine.machine_id)}
            onUpdate={(patch) => updateOverride(machine.machine_id, patch)}
          />
        );
      })}

      <button
        type="submit"
        disabled={saving}
        className="flex items-center gap-2 px-4 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all disabled:opacity-50"
      >
        <Save size={16} />
        {saving ? 'Saving...' : 'Save'}
      </button>
    </form>
  );
}

function MachineOverrideCard({
  machine,
  override,
  expanded,
  onToggle,
  onUpdate,
}: {
  machine: Machine;
  override: MachineOverride;
  expanded: boolean;
  onToggle: () => void;
  onUpdate: (patch: Partial<MachineOverride>) => void;
}) {
  return (
    <div className="bg-bg-secondary rounded-lg border border-border-primary overflow-hidden">
      <button
        type="button"
        onClick={onToggle}
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-bg-tertiary/40 transition-colors"
      >
        {expanded ? <ChevronDown size={16} className="text-text-secondary" /> : <ChevronRight size={16} className="text-text-secondary" />}
        <Server size={16} className="text-text-secondary" />
        <span className="text-sm font-medium text-text-primary flex-1">{machine.display_name || machine.machine_id}</span>
        <span className={`text-xs px-2 py-0.5 rounded-full ${machine.status === 'connected' ? 'bg-accent-green/10 text-accent-green' : 'bg-text-secondary/10 text-text-secondary'}`}>
          {machine.status}
        </span>
      </button>

      {expanded && (
        <div className="px-4 pb-4 pt-1 space-y-4 border-t border-border-primary">
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1.5">Working Directory</label>
            <input
              type="text"
              value={override.working_dir}
              onChange={(e) => onUpdate({ working_dir: e.target.value })}
              placeholder="/home/user/project"
              className="w-full px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-primary mb-1.5">Model</label>
            <select
              value={override.model}
              onChange={(e) => onUpdate({ model: e.target.value })}
              className="w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:ring-1 focus:ring-accent-primary"
            >
              {MODEL_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-text-primary mb-1.5">Environment Variables</label>
            <KeyValueEditor
              entries={envToEntries(override.env_vars)}
              onChange={(entries) => onUpdate({ env_vars: entriesToEnv(entries) })}
              keyPlaceholder="ENV_VAR"
              valuePlaceholder="value"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-primary mb-1.5">Max Concurrent Sessions</label>
            <input
              type="number"
              value={override.max_concurrent_sessions || ''}
              onChange={(e) => onUpdate({ max_concurrent_sessions: Number(e.target.value) || 0 })}
              min={0}
              placeholder="0 (use machine default)"
              className="w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
            />
            <p className="text-xs text-text-secondary mt-1">0 uses the machine default</p>
          </div>
        </div>
      )}
    </div>
  );
}
