import { useState, useMemo } from 'react';
import { Link, Pencil, Pause, Play, Plus, Trash2, Zap } from 'lucide-react';
import { toast } from 'sonner';
import { useTriggers, useCreateTrigger, useUpdateTrigger, useToggleTrigger, useDeleteTrigger } from '../../hooks/useTriggers.ts';
import { useJobs } from '../../hooks/useJobs.ts';
import { TriggerBuilder } from './TriggerBuilder.tsx';
import type { JobTrigger, CreateTriggerParams, UpdateTriggerParams } from '../../types/trigger.ts';

interface TriggerRowProps {
  trigger: JobTrigger;
  onEdit: (trigger: JobTrigger) => void;
  onToggle: (triggerId: string) => void;
  onDelete: (triggerId: string) => void;
  isDeleting: boolean;
  isToggling: boolean;
  jobNameMap: ReadonlyMap<string, string>;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

function parseFilterJobId(filter: string): string | null {
  if (!filter.trim()) return null;
  try {
    const parsed = JSON.parse(filter);
    if (typeof parsed === 'object' && parsed !== null && typeof parsed.job_id === 'string') {
      return parsed.job_id;
    }
  } catch {
    // not valid JSON, ignore
  }
  return null;
}

const EVENT_LABELS: Record<string, { verb: string; noun: string }> = {
  'run.completed': { verb: 'completes', noun: 'completion' },
  'run.failed': { verb: 'fails', noun: 'failure' },
  'run.started': { verb: 'starts', noun: 'start' },
  'run.created': { verb: 'is created', noun: 'creation' },
  'run.cancelled': { verb: 'is cancelled', noun: 'cancellation' },
};

function buildTriggerDescription(
  trigger: JobTrigger,
  jobNameMap: ReadonlyMap<string, string>,
): { text: string; isChained: boolean } {
  const filterJobId = parseFilterJobId(trigger.filter);
  const label = EVENT_LABELS[trigger.event_type];

  if (filterJobId && label) {
    const jobName = jobNameMap.get(filterJobId);
    const displayName = jobName ?? filterJobId.slice(0, 8);
    return {
      text: `Fires when ${displayName} ${label.verb}`,
      isChained: true,
    };
  }

  return {
    text: `Fires on ${trigger.event_type}`,
    isChained: false,
  };
}

function TriggerRow({ trigger, onEdit, onToggle, onDelete, isDeleting, isToggling, jobNameMap }: TriggerRowProps) {
  const [confirmDelete, setConfirmDelete] = useState(false);

  const description = buildTriggerDescription(trigger, jobNameMap);

  function handleDeleteClick() {
    if (confirmDelete) {
      onDelete(trigger.trigger_id);
    } else {
      setConfirmDelete(true);
    }
  }

  function handleCancelDelete() {
    setConfirmDelete(false);
  }

  return (
    <div className={`border border-border-primary rounded-md p-3 space-y-2 bg-bg-tertiary${!trigger.enabled ? ' opacity-50' : ''}`}>
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className={`text-xs text-text-primary flex items-center gap-1.5${!trigger.enabled ? ' line-through' : ''}`}>
            {description.isChained && <Link size={11} className="text-blue-400 shrink-0" />}
            <span className="truncate">{description.text}</span>
          </p>
          <p className={`text-xs font-mono text-text-secondary mt-0.5 truncate${!trigger.enabled ? ' line-through' : ''}`}>
            {trigger.event_type}
          </p>
          {trigger.filter && (
            <p className="text-xs text-text-secondary font-mono mt-0.5 truncate" title={trigger.filter}>
              filter: {trigger.filter}
            </p>
          )}
          <p className="text-xs text-text-secondary mt-0.5">
            Created {formatDate(trigger.created_at)}
          </p>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          {confirmDelete ? (
            <>
              <button
                onClick={handleDeleteClick}
                disabled={isDeleting}
                className="px-2 py-1 text-xs rounded-md bg-red-600 hover:bg-red-600/80 text-white transition-colors disabled:opacity-40"
              >
                Confirm
              </button>
              <button
                onClick={handleCancelDelete}
                className="px-2 py-1 text-xs rounded-md bg-bg-secondary text-text-secondary hover:text-text-primary transition-colors"
              >
                Cancel
              </button>
            </>
          ) : (
            <>
              <button
                onClick={() => onToggle(trigger.trigger_id)}
                disabled={isToggling}
                className="p-1 text-text-secondary hover:text-accent-primary transition-colors disabled:opacity-40"
                title={trigger.enabled ? 'Disable trigger' : 'Enable trigger'}
              >
                {trigger.enabled ? <Pause size={13} /> : <Play size={13} />}
              </button>
              <button
                onClick={() => onEdit(trigger)}
                className="p-1 text-text-secondary hover:text-accent-primary transition-colors"
                title="Edit trigger"
              >
                <Pencil size={13} />
              </button>
              <button
                onClick={handleDeleteClick}
                className="p-1 text-text-secondary hover:text-red-400 transition-colors"
                title="Delete trigger"
              >
                <Trash2 size={13} />
              </button>
            </>
          )}
        </div>
      </div>
      <span
        className={`inline-block text-xs px-1.5 py-0.5 rounded-full ${
          trigger.enabled
            ? 'bg-green-600/20 text-green-400'
            : 'bg-gray-600/20 text-gray-400'
        }`}
      >
        {trigger.enabled ? 'Enabled' : 'Disabled'}
      </span>
    </div>
  );
}

interface TriggerPanelProps {
  jobId: string;
}

export function TriggerPanel({ jobId }: TriggerPanelProps) {
  const { data: triggers, isLoading } = useTriggers(jobId);
  const { data: jobs } = useJobs();
  const createTrigger = useCreateTrigger();
  const updateTrigger = useUpdateTrigger();
  const toggleTrigger = useToggleTrigger();
  const deleteTrigger = useDeleteTrigger();

  const [showForm, setShowForm] = useState(false);
  const [editingTrigger, setEditingTrigger] = useState<JobTrigger | null>(null);

  const jobNameMap = useMemo(() => {
    const map = new Map<string, string>();
    if (jobs) {
      for (const job of jobs) {
        map.set(job.job_id, job.name);
      }
    }
    return map;
  }, [jobs]);

  async function handleCreate(params: CreateTriggerParams) {
    try {
      await createTrigger.mutateAsync({ jobId, params });
      toast.success('Trigger created');
      setShowForm(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create trigger');
    }
  }

  async function handleUpdate(params: UpdateTriggerParams) {
    if (!editingTrigger) return;
    try {
      await updateTrigger.mutateAsync({ triggerId: editingTrigger.trigger_id, params });
      toast.success('Trigger updated');
      setEditingTrigger(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update trigger');
    }
  }

  function handleEdit(trigger: JobTrigger) {
    setShowForm(false);
    setEditingTrigger(trigger);
  }

  function handleToggle(triggerId: string) {
    toggleTrigger.mutate(
      { triggerId, jobId },
      {
        onSuccess: (data) => toast.success(`Trigger ${data.enabled ? 'enabled' : 'disabled'}`),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to toggle trigger'),
      },
    );
  }

  function handleDelete(triggerId: string) {
    deleteTrigger.mutate(
      { triggerId, jobId },
      {
        onSuccess: () => toast.success('Trigger deleted'),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete trigger'),
      },
    );
  }

  function handleCancelForm() {
    setShowForm(false);
    setEditingTrigger(null);
  }

  function openCreateForm() {
    setEditingTrigger(null);
    setShowForm(true);
  }

  const isFormVisible = showForm || editingTrigger !== null;

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-primary">
        <span className="text-xs font-medium text-text-secondary flex items-center gap-1.5">
          <Zap size={13} />
          Triggers
        </span>
        {!isFormVisible && (
          <button
            onClick={openCreateForm}
            className="flex items-center gap-1 px-2 py-1 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
          >
            <Plus size={12} />
            Add
          </button>
        )}
      </div>

      <div className="flex-1 p-3 space-y-3">
        {showForm && (
          <TriggerBuilder
            onSave={handleCreate}
            onCancel={handleCancelForm}
            isSaving={createTrigger.isPending}
          />
        )}

        {editingTrigger && (
          <TriggerBuilder
            onSave={handleUpdate}
            onCancel={handleCancelForm}
            isSaving={updateTrigger.isPending}
            editingTrigger={editingTrigger}
          />
        )}

        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 2 }, (_, i) => (
              <div key={i} className="h-16 bg-bg-tertiary rounded-md animate-pulse" />
            ))}
          </div>
        ) : !triggers || triggers.length === 0 ? (
          <p className="text-xs text-text-secondary text-center py-4">
            No triggers yet. Add one to react to system events.
          </p>
        ) : (
          <div className="space-y-2">
            {triggers.map((trigger) => (
              <TriggerRow
                key={trigger.trigger_id}
                trigger={trigger}
                onEdit={handleEdit}
                onToggle={handleToggle}
                onDelete={handleDelete}
                isDeleting={deleteTrigger.isPending}
                isToggling={toggleTrigger.isPending}
                jobNameMap={jobNameMap}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
