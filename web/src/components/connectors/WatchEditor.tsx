import { Trash2 } from 'lucide-react';
import { useTemplates } from '../../hooks/useTemplates.ts';
import { TriggerConfig } from './TriggerConfig.tsx';
import type { TriggerFilters, TriggerType } from './TriggerConfig.tsx';

export interface WatchData {
  id: string;  // stable unique key for React rendering
  repo: string;
  template: string;
  machine_id: string;
  poll_interval: string;
  triggers: Record<TriggerType, { enabled: boolean; filters: TriggerFilters }>;
}

export interface WatchEditorProps {
  watch: WatchData;
  onChange: (watch: WatchData) => void;
  onRemove: () => void;
  index: number;
}

const POLL_INTERVAL_OPTIONS = [
  { value: '30s', label: '30 seconds' },
  { value: '60s', label: '1 minute' },
  { value: '120s', label: '2 minutes' },
  { value: '300s', label: '5 minutes' },
];

const TRIGGER_TYPES: TriggerType[] = [
  'pull_request_opened',
  'check_run_completed',
  'issue_labeled',
  'issue_comment',
  'pull_request_comment',
  'pull_request_review',
  'release_published',
];

const inputClass =
  'w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30';

const selectClass =
  'w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary';

function repoFormatValid(repo: string): boolean {
  if (repo === '') return true;
  return /^[^/\s]+\/[^/\s]+$/.test(repo);
}

export function WatchEditor({ watch, onChange, onRemove, index }: WatchEditorProps) {
  const { data: templates } = useTemplates();

  function update<K extends keyof WatchData>(key: K, value: WatchData[K]) {
    onChange({ ...watch, [key]: value });
  }

  function updateTriggerEnabled(type: TriggerType, enabled: boolean) {
    onChange({
      ...watch,
      triggers: {
        ...watch.triggers,
        [type]: { ...watch.triggers[type], enabled },
      },
    });
  }

  function updateTriggerFilters(type: TriggerType, filters: TriggerFilters) {
    onChange({
      ...watch,
      triggers: {
        ...watch.triggers,
        [type]: { ...watch.triggers[type], filters },
      },
    });
  }

  const repoValid = repoFormatValid(watch.repo);

  return (
    <div className="rounded-lg border border-border-primary bg-bg-secondary">
      {/* Card header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border-primary">
        <span className="text-sm font-semibold text-text-primary">Watch #{index + 1}</span>
        <button
          type="button"
          onClick={onRemove}
          className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
          aria-label="Remove watch"
        >
          <Trash2 size={14} />
        </button>
      </div>

      <div className="px-4 py-4 flex flex-col gap-4">
        {/* Repository */}
        <div>
          <label className="block text-sm text-text-secondary mb-1">
            Repository <span className="text-status-error">*</span>
          </label>
          <input
            type="text"
            value={watch.repo}
            onChange={(e) => update('repo', e.target.value)}
            placeholder="owner/repo"
            className={`${inputClass} ${!repoValid ? 'border-status-error focus:ring-status-error' : ''}`}
          />
          {!repoValid && (
            <p className="mt-1 text-xs text-status-error">
              Use &ldquo;owner/repo&rdquo; format, e.g. acme/my-service
            </p>
          )}
        </div>

        {/* Template */}
        <div>
          <label className="block text-sm text-text-secondary mb-1">
            Template <span className="text-status-error">*</span>
          </label>
          <select
            value={watch.template}
            onChange={(e) => update('template', e.target.value)}
            className={selectClass}
          >
            <option value="">Select a template…</option>
            {(templates ?? []).map((t) => (
              <option key={t.template_id} value={t.name}>
                {t.name}
              </option>
            ))}
          </select>
        </div>

        {/* Machine ID + Poll Interval */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="block text-sm text-text-secondary mb-1">Machine ID</label>
            <input
              type="text"
              value={watch.machine_id}
              onChange={(e) => update('machine_id', e.target.value)}
              placeholder="Any available machine"
              className={inputClass}
            />
          </div>
          <div>
            <label className="block text-sm text-text-secondary mb-1">Poll interval</label>
            <select
              value={watch.poll_interval}
              onChange={(e) => update('poll_interval', e.target.value)}
              className={selectClass}
            >
              {POLL_INTERVAL_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Triggers */}
        <div>
          <p className="text-sm text-text-secondary mb-2">Triggers</p>
          <div className="flex flex-col gap-2">
            {TRIGGER_TYPES.map((type) => (
              <TriggerConfig
                key={type}
                type={type}
                enabled={watch.triggers[type].enabled}
                onToggle={(enabled) => updateTriggerEnabled(type, enabled)}
                filters={watch.triggers[type].filters}
                onFiltersChange={(filters) => updateTriggerFilters(type, filters)}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

