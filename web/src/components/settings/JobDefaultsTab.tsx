import { useState, useCallback } from 'react';
import { Save } from 'lucide-react';
import { toast } from 'sonner';
import type { UserPreferences } from '../../types/preferences.ts';

const DEFAULT_STEP_DELAY_MAX_SECONDS = 86400;

interface JobDefaultsTabProps {
  preferences: UserPreferences;
  onSave: (prefs: UserPreferences) => Promise<void>;
  saving: boolean;
}

export function JobDefaultsTab({ preferences, onSave, saving }: JobDefaultsTabProps) {
  const [stepTimeout, setStepTimeout] = useState(String(preferences.default_step_timeout ?? ''));
  const [stepDelay, setStepDelay] = useState(String(preferences.default_step_delay ?? ''));

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    const delayNum = stepDelay ? Number(stepDelay) : undefined;
    if (delayNum !== undefined && (!Number.isFinite(delayNum) || delayNum < 0 || delayNum > DEFAULT_STEP_DELAY_MAX_SECONDS)) {
      toast.error(`Step delay must be between 0 and ${DEFAULT_STEP_DELAY_MAX_SECONDS} seconds`);
      return;
    }
    try {
      await onSave({
        ...preferences,
        default_step_timeout: stepTimeout ? Number(stepTimeout) : undefined,
        default_step_delay: delayNum,
      });
      toast.success('Job defaults saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  }, [preferences, stepTimeout, stepDelay, onSave]);

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">
          Default Step Timeout (seconds)
        </label>
        <input
          type="number"
          value={stepTimeout}
          onChange={(e) => setStepTimeout(e.target.value)}
          min={0}
          placeholder="No timeout"
          className="w-full sm:w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
        />
        <p className="text-xs text-text-secondary mt-1">Leave empty for no timeout</p>
      </div>

      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">
          Default Step Delay (seconds)
        </label>
        <input
          type="number"
          value={stepDelay}
          onChange={(e) => setStepDelay(e.target.value)}
          min={0}
          max={DEFAULT_STEP_DELAY_MAX_SECONDS}
          placeholder="0"
          className="w-full sm:w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
        />
        <p className="text-xs text-text-secondary mt-1">
          Delay between steps (0-{DEFAULT_STEP_DELAY_MAX_SECONDS})
        </p>
      </div>

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
