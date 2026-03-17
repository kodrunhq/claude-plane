import { useState, useMemo } from 'react';
import { Info, Link, Save, X } from 'lucide-react';
import type { CreateTriggerParams, JobTrigger } from '../../types/trigger.ts';
import { KNOWN_EVENT_TYPES } from '../../types/trigger.ts';
import { useJobs } from '../../hooks/useJobs.ts';

interface TriggerBuilderProps {
  onSave: (params: CreateTriggerParams) => Promise<void>;
  onCancel: () => void;
  isSaving: boolean;
  editingTrigger?: JobTrigger;
}

interface FormState {
  event_type: string;
  custom_event_type: string;
  filter: string;
  source_job_id: string;
}

const CUSTOM_OPTION = '__custom__';
const NO_JOB_SELECTED = '';

const JOB_CHAINING_EVENT_TYPES = ['run.completed', 'run.failed'];

const DEFAULT_FORM: FormState = {
  event_type: KNOWN_EVENT_TYPES[0],
  custom_event_type: '',
  filter: '',
  source_job_id: NO_JOB_SELECTED,
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

function isJobChainingEvent(eventType: string): boolean {
  return JOB_CHAINING_EVENT_TYPES.includes(eventType);
}

function buildInitialForm(trigger?: JobTrigger): FormState {
  if (!trigger) return DEFAULT_FORM;

  const knownTypes: readonly string[] = KNOWN_EVENT_TYPES;
  const isKnown = knownTypes.includes(trigger.event_type);
  const sourceJobId = parseSourceJobId(trigger.filter);

  return {
    event_type: isKnown ? trigger.event_type : CUSTOM_OPTION,
    custom_event_type: isKnown ? '' : trigger.event_type,
    filter: trigger.filter,
    source_job_id: sourceJobId ?? NO_JOB_SELECTED,
  };
}

function parseSourceJobId(filter: string): string | null {
  if (!filter.trim()) return null;
  try {
    const parsed = JSON.parse(filter);
    if (typeof parsed === 'object' && parsed !== null && typeof parsed.job_id === 'string') {
      return parsed.job_id;
    }
  } catch {
    // not valid JSON
  }
  return null;
}

export function TriggerBuilder({ onSave, onCancel, isSaving, editingTrigger }: TriggerBuilderProps) {
  const [form, setForm] = useState<FormState>(() => buildInitialForm(editingTrigger));
  const { data: jobs } = useJobs();

  const isCustom = form.event_type === CUSTOM_OPTION;
  const resolvedEventType = isCustom ? form.custom_event_type.trim() : form.event_type;
  const showJobChaining = isJobChainingEvent(resolvedEventType);
  const filterValid = isValidJson(form.filter);
  const canSave = resolvedEventType.length > 0 && filterValid;

  const sortedJobs = useMemo(() => {
    if (!jobs) return [];
    return [...jobs].sort((a, b) => a.name.localeCompare(b.name));
  }, [jobs]);

  function handleEventTypeChange(value: string) {
    setForm((prev) => ({
      ...prev,
      event_type: value,
      source_job_id: NO_JOB_SELECTED,
    }));
  }

  function handleSourceJobChange(jobId: string) {
    if (jobId === NO_JOB_SELECTED) {
      setForm((prev) => ({ ...prev, source_job_id: jobId, filter: '' }));
      return;
    }
    const filterJson = JSON.stringify({ job_id: jobId }, null, 2);
    setForm((prev) => ({ ...prev, source_job_id: jobId, filter: filterJson }));
  }

  function handleFilterChange(value: string) {
    setForm((prev) => ({ ...prev, filter: value, source_job_id: NO_JOB_SELECTED }));
  }

  async function handleSave() {
    if (!canSave) return;
    await onSave({ event_type: resolvedEventType, filter: form.filter.trim() || '' });
  }

  return (
    <div className="border border-border-primary rounded-md p-3 space-y-3 bg-bg-tertiary">
      <div className="space-y-1">
        <label className="text-xs text-text-secondary">Event Type</label>
        <select
          value={form.event_type}
          onChange={(e) => handleEventTypeChange(e.target.value)}
          className="w-full bg-bg-secondary border border-border-primary rounded-md px-2 py-1.5 text-sm text-text-primary focus:outline-none focus:border-accent-primary"
        >
          {KNOWN_EVENT_TYPES.map((et) => (
            <option key={et} value={et}>
              {et}
            </option>
          ))}
          <option value={CUSTOM_OPTION}>Custom (type below)</option>
        </select>
        {isCustom && (
          <input
            type="text"
            value={form.custom_event_type}
            placeholder="e.g. run.completed"
            className="w-full bg-bg-secondary border border-border-primary rounded-md px-2 py-1.5 text-sm text-text-primary font-mono focus:outline-none focus:border-accent-primary"
            onChange={(e) => setForm((prev) => ({ ...prev, custom_event_type: e.target.value }))}
          />
        )}
      </div>

      {showJobChaining && (
        <div className="space-y-2">
          <div className="flex items-start gap-2 rounded-md bg-blue-500/10 border border-blue-500/20 px-3 py-2">
            <Info size={14} className="text-blue-400 mt-0.5 shrink-0" />
            <p className="text-xs text-blue-300">
              To trigger this job when a specific job {resolvedEventType === 'run.failed' ? 'fails' : 'completes'},
              select the source job below or add a filter with its ID.
            </p>
          </div>

          <div className="space-y-1">
            <label className="text-xs text-text-secondary flex items-center gap-1">
              <Link size={11} />
              Source Job
            </label>
            <select
              value={form.source_job_id}
              onChange={(e) => handleSourceJobChange(e.target.value)}
              className="w-full bg-bg-secondary border border-border-primary rounded-md px-2 py-1.5 text-sm text-text-primary focus:outline-none focus:border-accent-primary"
            >
              <option value={NO_JOB_SELECTED}>None (any job)</option>
              {sortedJobs.map((job) => (
                <option key={job.job_id} value={job.job_id}>
                  {job.name}
                </option>
              ))}
            </select>
          </div>
        </div>
      )}

      <div className="space-y-1">
        <label className="text-xs text-text-secondary">Filter (optional JSON)</label>
        <textarea
          value={form.filter}
          onChange={(e) => handleFilterChange(e.target.value)}
          rows={3}
          placeholder='e.g. {"status": "completed"}'
          className={`w-full bg-bg-secondary border rounded-md px-2 py-1.5 text-sm text-text-primary font-mono focus:outline-none resize-none ${
            filterValid ? 'border-border-primary focus:border-accent-primary' : 'border-red-500'
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
          {isSaving ? 'Saving...' : editingTrigger ? 'Update' : 'Save'}
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
