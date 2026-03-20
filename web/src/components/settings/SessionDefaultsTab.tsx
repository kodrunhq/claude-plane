import { useState, useCallback } from 'react';
import { Save } from 'lucide-react';
import { toast } from 'sonner';
import { KeyValueEditor } from './KeyValueEditor.tsx';
import { envToEntries, entriesToEnv } from './envUtils.ts';
import type { UserPreferences } from '../../types/preferences.ts';

interface SessionDefaultsTabProps {
  preferences: UserPreferences;
  onSave: (prefs: UserPreferences) => Promise<void>;
  saving: boolean;
}

export function SessionDefaultsTab({ preferences, onSave, saving }: SessionDefaultsTabProps) {
  const [skipPermissions, setSkipPermissions] = useState(preferences.skip_permissions ?? true);
  const [sessionTimeout, setSessionTimeout] = useState(String(preferences.default_session_timeout ?? ''));
  const [staleTimeout, setStaleTimeout] = useState(String(preferences.session_stale_timeout ?? ''));
  const [envEntries, setEnvEntries] = useState(envToEntries(preferences.default_env_vars));

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await onSave({
        ...preferences,
        skip_permissions: skipPermissions,
        default_session_timeout: sessionTimeout ? Number(sessionTimeout) : undefined,
        session_stale_timeout: staleTimeout ? Number(staleTimeout) : undefined,
        default_env_vars: Object.keys(entriesToEnv(envEntries)).length > 0 ? entriesToEnv(envEntries) : undefined,
      });
      toast.success('Session defaults saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  }, [preferences, skipPermissions, sessionTimeout, staleTimeout, envEntries, onSave]);

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div>
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={skipPermissions}
            onChange={(e) => setSkipPermissions(e.target.checked)}
            className="w-4 h-4 rounded border-border-primary bg-bg-tertiary text-accent-primary focus:ring-accent-primary"
          />
          <div>
            <p className="text-sm font-medium text-text-primary">Skip Permissions</p>
            <p className="text-xs text-text-secondary">Allow sessions to run without permission prompts</p>
          </div>
        </label>
      </div>

      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">
          Default Session Timeout (seconds)
        </label>
        <input
          type="number"
          value={sessionTimeout}
          onChange={(e) => setSessionTimeout(e.target.value)}
          min={0}
          placeholder="No timeout"
          className="w-full sm:w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
        />
        <p className="text-xs text-text-secondary mt-1">Leave empty for no timeout</p>
      </div>

      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">
          Session Idle Timeout (minutes)
        </label>
        <input
          type="number"
          value={staleTimeout}
          onChange={(e) => setStaleTimeout(e.target.value)}
          min={0}
          placeholder="4320 (3 days)"
          className="w-full sm:w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
        />
        <p className="text-xs text-text-secondary mt-1">
          Auto-terminate idle standalone sessions after this many minutes. Default: 3 days (4320 min). Leave empty for no auto-terminate.
        </p>
      </div>

      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">
          Default Environment Variables
        </label>
        <KeyValueEditor
          entries={envEntries}
          onChange={setEnvEntries}
          keyPlaceholder="ENV_VAR"
          valuePlaceholder="value"
        />
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
