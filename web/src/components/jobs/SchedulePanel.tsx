import { useState, useMemo } from 'react';
import { Plus, Trash2, Pause, Play, Clock, Calendar, X, Save } from 'lucide-react';
import { toast } from 'sonner';
import cronstrue from 'cronstrue';
import { CronExpressionParser } from 'cron-parser';
import {
  useSchedules,
  useCreateSchedule,
  useUpdateSchedule,
  useDeleteSchedule,
  usePauseSchedule,
  useResumeSchedule,
} from '../../hooks/useSchedules.ts';
import type { CronSchedule, CreateScheduleParams } from '../../types/schedule.ts';

const COMMON_TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Asia/Tokyo',
  'Asia/Shanghai',
  'Asia/Kolkata',
  'Australia/Sydney',
];

interface ScheduleFormState {
  cron_expr: string;
  timezone: string;
}

const DEFAULT_FORM: ScheduleFormState = {
  cron_expr: '0 9 * * MON',
  timezone: 'UTC',
};

function parseCronDescription(expr: string): string {
  try {
    return cronstrue.toString(expr, { throwExceptionOnParseError: true });
  } catch {
    return 'Invalid cron expression';
  }
}

function getNextRuns(expr: string, tz: string, count: number): Date[] {
  try {
    const interval = CronExpressionParser.parse(expr, { tz });
    const runs: Date[] = [];
    for (let i = 0; i < count; i++) {
      runs.push(interval.next().toDate());
    }
    return runs;
  } catch {
    return [];
  }
}

function formatDateTime(date: Date, tz?: string): string {
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: tz,
    timeZoneName: 'short',
  });
}

interface ScheduleFormProps {
  initial: ScheduleFormState;
  onSave: (params: CreateScheduleParams) => Promise<void>;
  onCancel: () => void;
  isSaving: boolean;
}

function ScheduleForm({ initial, onSave, onCancel, isSaving }: ScheduleFormProps) {
  const [form, setForm] = useState<ScheduleFormState>(initial);

  const description = useMemo(() => parseCronDescription(form.cron_expr), [form.cron_expr]);
  const nextRuns = useMemo(
    () => getNextRuns(form.cron_expr, form.timezone, 5),
    [form.cron_expr, form.timezone],
  );

  const isValid = nextRuns.length > 0;

  async function handleSave() {
    if (!isValid) return;
    await onSave({ cron_expr: form.cron_expr, timezone: form.timezone });
  }

  return (
    <div className="border border-border-primary rounded-md p-3 space-y-3 bg-bg-tertiary">
      <div className="space-y-1">
        <label className="text-xs text-text-secondary">Cron Expression</label>
        <input
          type="text"
          value={form.cron_expr}
          onChange={(e) => setForm((prev) => ({ ...prev, cron_expr: e.target.value }))}
          className="w-full bg-bg-secondary border border-border-primary rounded-md px-2 py-1.5 text-sm text-text-primary focus:outline-none focus:border-accent-primary font-mono"
          placeholder="0 9 * * MON"
        />
        <p className={`text-xs ${isValid ? 'text-text-secondary' : 'text-red-400'}`}>
          {description}
        </p>
      </div>

      <div className="space-y-1">
        <label className="text-xs text-text-secondary">Timezone</label>
        <select
          value={form.timezone}
          onChange={(e) => setForm((prev) => ({ ...prev, timezone: e.target.value }))}
          className="w-full bg-bg-secondary border border-border-primary rounded-md px-2 py-1.5 text-sm text-text-primary focus:outline-none focus:border-accent-primary"
        >
          {COMMON_TIMEZONES.map((tz) => (
            <option key={tz} value={tz}>
              {tz}
            </option>
          ))}
        </select>
      </div>

      {nextRuns.length > 0 && (
        <div className="space-y-1">
          <p className="text-xs text-text-secondary flex items-center gap-1">
            <Calendar size={11} />
            Next 5 runs
          </p>
          <ul className="space-y-0.5">
            {nextRuns.map((d, i) => (
              <li key={i} className="text-xs text-text-secondary pl-1">
                {formatDateTime(d, form.timezone)}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="flex items-center gap-2 pt-1">
        <button
          onClick={handleSave}
          disabled={!isValid || isSaving}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Save size={12} />
          {isSaving ? 'Saving…' : 'Save'}
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

interface ScheduleRowProps {
  schedule: CronSchedule;
  onPause: (id: string) => void;
  onResume: (id: string) => void;
  onDelete: (id: string) => void;
  onEdit: (schedule: CronSchedule) => void;
  isToggling: boolean;
}

function ScheduleRow({ schedule, onPause, onResume, onDelete, onEdit, isToggling }: ScheduleRowProps) {
  const description = useMemo(
    () => parseCronDescription(schedule.cron_expr),
    [schedule.cron_expr],
  );

  return (
    <div className="border border-border-primary rounded-md p-3 space-y-1 bg-bg-tertiary">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className="text-xs font-mono text-text-primary truncate">{schedule.cron_expr}</p>
          <p className="text-xs text-text-secondary mt-0.5">{description}</p>
          <p className="text-xs text-text-secondary">{schedule.timezone}</p>
          {schedule.next_run_at && (
            <p className="text-xs text-text-secondary mt-0.5">
              Next: {formatDateTime(new Date(schedule.next_run_at), schedule.timezone)}
            </p>
          )}
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button
            onClick={() => onEdit(schedule)}
            className="p-1 text-text-secondary hover:text-text-primary transition-colors"
            title="Edit"
          >
            <Save size={13} />
          </button>
          <button
            onClick={() => (schedule.enabled ? onPause(schedule.schedule_id) : onResume(schedule.schedule_id))}
            disabled={isToggling}
            className="p-1 text-text-secondary hover:text-text-primary transition-colors disabled:opacity-40"
            title={schedule.enabled ? 'Pause' : 'Resume'}
          >
            {schedule.enabled ? <Pause size={13} /> : <Play size={13} />}
          </button>
          <button
            onClick={() => onDelete(schedule.schedule_id)}
            className="p-1 text-text-secondary hover:text-red-400 transition-colors"
            title="Delete"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>
      <span
        className={`inline-block text-xs px-1.5 py-0.5 rounded-full ${
          schedule.enabled
            ? 'bg-green-600/20 text-green-400'
            : 'bg-gray-600/20 text-gray-400'
        }`}
      >
        {schedule.enabled ? 'Active' : 'Paused'}
      </span>
    </div>
  );
}

interface SchedulePanelProps {
  jobId: string;
}

type FormMode = 'hidden' | 'create' | { editId: string; initial: ScheduleFormState };

export function SchedulePanel({ jobId }: SchedulePanelProps) {
  const { data: schedules, isLoading } = useSchedules(jobId);
  const createSchedule = useCreateSchedule();
  const updateSchedule = useUpdateSchedule();
  const deleteSchedule = useDeleteSchedule();
  const pauseSchedule = usePauseSchedule();
  const resumeSchedule = useResumeSchedule();

  const [formMode, setFormMode] = useState<FormMode>('hidden');

  async function handleCreate(params: CreateScheduleParams) {
    try {
      await createSchedule.mutateAsync({ jobId, params });
      toast.success('Schedule created');
      setFormMode('hidden');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create schedule');
    }
  }

  async function handleUpdate(params: CreateScheduleParams) {
    if (typeof formMode !== 'object') return;
    try {
      await updateSchedule.mutateAsync({ id: formMode.editId, params });
      toast.success('Schedule updated');
      setFormMode('hidden');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update schedule');
    }
  }

  async function handleDelete(id: string) {
    try {
      await deleteSchedule.mutateAsync({ id, jobId });
      toast.success('Schedule deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete schedule');
    }
  }

  async function handlePause(id: string) {
    try {
      await pauseSchedule.mutateAsync(id);
      toast.success('Schedule paused');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to pause schedule');
    }
  }

  async function handleResume(id: string) {
    try {
      await resumeSchedule.mutateAsync(id);
      toast.success('Schedule resumed');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to resume schedule');
    }
  }

  function handleEditClick(schedule: CronSchedule) {
    setFormMode({
      editId: schedule.schedule_id,
      initial: { cron_expr: schedule.cron_expr, timezone: schedule.timezone },
    });
  }

  const isSaving = createSchedule.isPending || updateSchedule.isPending;
  const isToggling = pauseSchedule.isPending || resumeSchedule.isPending;

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-primary">
        <span className="text-xs font-medium text-text-secondary flex items-center gap-1.5">
          <Clock size={13} />
          Schedules
        </span>
        {formMode === 'hidden' && (
          <button
            onClick={() => setFormMode('create')}
            className="flex items-center gap-1 px-2 py-1 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
          >
            <Plus size={12} />
            Add
          </button>
        )}
      </div>

      <div className="flex-1 p-3 space-y-3">
        {formMode === 'create' && (
          <ScheduleForm
            initial={DEFAULT_FORM}
            onSave={handleCreate}
            onCancel={() => setFormMode('hidden')}
            isSaving={isSaving}
          />
        )}

        {typeof formMode === 'object' && (
          <ScheduleForm
            initial={formMode.initial}
            onSave={handleUpdate}
            onCancel={() => setFormMode('hidden')}
            isSaving={isSaving}
          />
        )}

        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 2 }, (_, i) => (
              <div key={i} className="h-16 bg-bg-tertiary rounded-md animate-pulse" />
            ))}
          </div>
        ) : !schedules || schedules.length === 0 ? (
          <p className="text-xs text-text-secondary text-center py-4">
            No schedules yet. Add one to automate this job.
          </p>
        ) : (
          <div className="space-y-2">
            {schedules.map((schedule) => (
              <ScheduleRow
                key={schedule.schedule_id}
                schedule={schedule}
                onPause={handlePause}
                onResume={handleResume}
                onDelete={handleDelete}
                onEdit={handleEditClick}
                isToggling={isToggling}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
