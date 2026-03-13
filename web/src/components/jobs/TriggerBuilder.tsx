import { useState } from 'react';
import { Save, X } from 'lucide-react';
import type { CreateTriggerParams } from '../../types/trigger.ts';
import { KNOWN_EVENT_TYPES } from '../../types/trigger.ts';

interface TriggerBuilderProps {
  onSave: (params: CreateTriggerParams) => Promise<void>;
  onCancel: () => void;
  isSaving: boolean;
}

interface FormState {
  event_type: string;
  filter: string;
}

const DEFAULT_FORM: FormState = {
  event_type: KNOWN_EVENT_TYPES[0],
  filter: '',
};

function isValidJson(value: string): boolean {
  if (!value.trim()) return true;
  try {
    JSON.parse(value);
    return true;
  } catch {
    return false;
  }
}

export function TriggerBuilder({ onSave, onCancel, isSaving }: TriggerBuilderProps) {
  const [form, setForm] = useState<FormState>(DEFAULT_FORM);

  const filterValid = isValidJson(form.filter);
  const canSave = form.event_type.trim().length > 0 && filterValid;

  function handleEventTypeChange(value: string) {
    setForm((prev) => ({ ...prev, event_type: value }));
  }

  function handleFilterChange(value: string) {
    setForm((prev) => ({ ...prev, filter: value }));
  }

  async function handleSave() {
    if (!canSave) return;
    await onSave({ event_type: form.event_type.trim(), filter: form.filter });
  }

  return (
    <div className="border border-gray-700 rounded-md p-3 space-y-3 bg-bg-tertiary">
      <div className="space-y-1">
        <label className="text-xs text-text-secondary">Event Type</label>
        <select
          value={form.event_type}
          onChange={(e) => handleEventTypeChange(e.target.value)}
          className="w-full bg-bg-secondary border border-gray-700 rounded-md px-2 py-1.5 text-sm text-text-primary focus:outline-none focus:border-accent-primary"
        >
          {KNOWN_EVENT_TYPES.map((et) => (
            <option key={et} value={et}>
              {et}
            </option>
          ))}
          <option value="custom">Custom (type below)</option>
        </select>
        {form.event_type === 'custom' && (
          <input
            type="text"
            placeholder="e.g. run.completed"
            className="w-full bg-bg-secondary border border-gray-700 rounded-md px-2 py-1.5 text-sm text-text-primary font-mono focus:outline-none focus:border-accent-primary"
            onChange={(e) => handleEventTypeChange(e.target.value)}
          />
        )}
      </div>

      <div className="space-y-1">
        <label className="text-xs text-text-secondary">Filter (optional JSON)</label>
        <textarea
          value={form.filter}
          onChange={(e) => handleFilterChange(e.target.value)}
          rows={3}
          placeholder='e.g. {"status": "completed"}'
          className={`w-full bg-bg-secondary border rounded-md px-2 py-1.5 text-sm text-text-primary font-mono focus:outline-none resize-none ${
            filterValid ? 'border-gray-700 focus:border-accent-primary' : 'border-red-500'
          }`}
        />
        {!filterValid && (
          <p className="text-xs text-red-400">Invalid JSON</p>
        )}
        {filterValid && form.filter.trim() && (
          <p className="text-xs text-text-secondary">Valid JSON filter</p>
        )}
      </div>

      <div className="flex items-center gap-2 pt-1">
        <button
          onClick={handleSave}
          disabled={!canSave || isSaving}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Save size={12} />
          {isSaving ? 'Saving...' : 'Save'}
        </button>
        <button
          onClick={onCancel}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-secondary text-text-secondary hover:text-text-primary transition-colors"
        >
          <X size={12} />
          Cancel
        </button>
      </div>
    </div>
  );
}
