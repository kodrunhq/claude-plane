import { useState, useCallback } from 'react';
import { Settings, Terminal, Workflow, Bell, Monitor, Server, Database, AlertCircle, RefreshCw } from 'lucide-react';
import { usePreferences, useUpdatePreferences } from '../hooks/usePreferences.ts';
import { useServerSettings, useUpdateServerSettings } from '../hooks/useSettings.ts';
import { SessionDefaultsTab } from '../components/settings/SessionDefaultsTab.tsx';
import { JobDefaultsTab } from '../components/settings/JobDefaultsTab.tsx';
import { NotificationsTab } from '../components/settings/NotificationsTab.tsx';
import { UIPreferencesTab } from '../components/settings/UIPreferencesTab.tsx';
import { MachinesTab } from '../components/settings/MachinesTab.tsx';
import type { UserPreferences } from '../types/preferences.ts';

const TABS = [
  { id: 'sessions', label: 'Session Defaults', icon: Terminal },
  { id: 'jobs', label: 'Job Defaults', icon: Workflow },
  { id: 'notifications', label: 'Notifications', icon: Bell },
  { id: 'ui', label: 'UI Preferences', icon: Monitor },
  { id: 'machines', label: 'Machines', icon: Server },
  { id: 'retention', label: 'Data Retention', icon: Database },
] as const;

type TabId = (typeof TABS)[number]['id'];

const RETENTION_OPTIONS = [
  { value: '7', label: '7 days' },
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' },
  { value: '365', label: '1 year' },
  { value: '0', label: 'Unlimited' },
];

function DataRetentionTab() {
  const { data: settings, isLoading, error, refetch } = useServerSettings();
  const updateSettings = useUpdateServerSettings();
  const currentValue = settings?.retention_days ?? '30';

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="h-8 w-48 bg-bg-tertiary rounded animate-pulse" />
        <div className="h-10 w-64 bg-bg-tertiary rounded animate-pulse" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load server settings'}
          </p>
          <button
            onClick={() => refetch()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold text-text-primary mb-1">Data Retention</h2>
        <p className="text-sm text-text-secondary">
          Configure how long terminal output is stored on the server and agent machines.
        </p>
      </div>

      <div className="space-y-2">
        <label htmlFor="retention-days" className="block text-sm font-medium text-text-primary">
          Retention Period
        </label>
        <select
          id="retention-days"
          value={currentValue}
          onChange={(e) => updateSettings.mutate({ retention_days: e.target.value })}
          disabled={updateSettings.isPending}
          className="block w-64 px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-accent-primary/50 disabled:opacity-50"
        >
          {RETENTION_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        {updateSettings.isError && (
          <p className="text-xs text-status-error mt-1">
            {updateSettings.error instanceof Error ? updateSettings.error.message : 'Failed to update setting'}
          </p>
        )}
        <p className="text-xs text-text-tertiary mt-1">
          Terminal output older than this is deleted from the server and agent machines.
          Running sessions are never affected.
        </p>
      </div>
    </div>
  );
}

export function SettingsPage() {
  const [activeTab, setActiveTab] = useState<TabId>('sessions');
  const { data: preferences, isLoading, error, refetch } = usePreferences();
  const updatePreferences = useUpdatePreferences();

  const handleSave = useCallback(async (prefs: UserPreferences) => {
    await updatePreferences.mutateAsync(prefs);
  }, [updatePreferences]);

  if (error) {
    return (
      <div className="p-4 md:p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load preferences'}
          </p>
          <button
            onClick={() => refetch()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (isLoading || !preferences) {
    return (
      <div className="p-4 md:p-6 space-y-6">
        <div className="flex items-center gap-3">
          <div className="h-6 w-32 bg-bg-tertiary rounded animate-pulse" />
        </div>
        <div className="h-10 w-full bg-bg-tertiary rounded-lg animate-pulse" />
        <div className="space-y-4">
          <div className="h-8 w-48 bg-bg-tertiary rounded animate-pulse" />
          <div className="h-8 w-64 bg-bg-tertiary rounded animate-pulse" />
          <div className="h-8 w-56 bg-bg-tertiary rounded animate-pulse" />
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 md:p-6 space-y-6">
      <div className="flex items-center gap-3">
        <Settings size={20} className="text-text-secondary" />
        <h1 className="text-xl font-semibold text-text-primary">Settings</h1>
      </div>

      {/* Tab bar */}
      <div className="flex gap-1 p-1 bg-bg-secondary rounded-lg border border-border-primary overflow-x-auto">
        {TABS.map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 px-4 py-2 text-sm rounded-md font-medium whitespace-nowrap transition-all ${
                isActive
                  ? 'bg-accent-primary/10 text-accent-primary'
                  : 'text-text-secondary hover:text-text-primary hover:bg-bg-tertiary/60'
              }`}
            >
              <Icon size={16} />
              {tab.label}
            </button>
          );
        })}
      </div>

      {/* Tab content */}
      <div className="bg-bg-secondary rounded-lg border border-border-primary p-6">
        {activeTab === 'sessions' && (
          <SessionDefaultsTab
            preferences={preferences}
            onSave={handleSave}
            saving={updatePreferences.isPending}
          />
        )}
        {activeTab === 'jobs' && (
          <JobDefaultsTab
            preferences={preferences}
            onSave={handleSave}
            saving={updatePreferences.isPending}
          />
        )}
        {activeTab === 'notifications' && (
          <NotificationsTab
            preferences={preferences}
            onSave={handleSave}
            saving={updatePreferences.isPending}
          />
        )}
        {activeTab === 'ui' && (
          <UIPreferencesTab
            preferences={preferences}
            onSave={handleSave}
            saving={updatePreferences.isPending}
          />
        )}
        {activeTab === 'machines' && (
          <MachinesTab
            preferences={preferences}
            onSave={handleSave}
            saving={updatePreferences.isPending}
          />
        )}
        {activeTab === 'retention' && <DataRetentionTab />}
      </div>
    </div>
  );
}
