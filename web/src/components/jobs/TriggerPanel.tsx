import { useState } from 'react';
import { Plus, Trash2, Zap } from 'lucide-react';
import { toast } from 'sonner';
import { useTriggers, useCreateTrigger, useDeleteTrigger } from '../../hooks/useTriggers.ts';
import { TriggerBuilder } from './TriggerBuilder.tsx';
import type { JobTrigger, CreateTriggerParams } from '../../types/trigger.ts';

interface TriggerRowProps {
  trigger: JobTrigger;
  onDelete: (triggerId: string) => void;
  isDeleting: boolean;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}

function TriggerRow({ trigger, onDelete, isDeleting }: TriggerRowProps) {
  const [confirmDelete, setConfirmDelete] = useState(false);

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
    <div className="border border-gray-700 rounded-md p-3 space-y-2 bg-bg-tertiary">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1 min-w-0">
          <p className="text-xs font-mono text-text-primary truncate">{trigger.event_type}</p>
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
            <button
              onClick={handleDeleteClick}
              className="p-1 text-text-secondary hover:text-red-400 transition-colors"
              title="Delete trigger"
            >
              <Trash2 size={13} />
            </button>
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
  const createTrigger = useCreateTrigger();
  const deleteTrigger = useDeleteTrigger();

  const [showForm, setShowForm] = useState(false);

  async function handleCreate(params: CreateTriggerParams) {
    try {
      await createTrigger.mutateAsync({ jobId, params });
      toast.success('Trigger created');
      setShowForm(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create trigger');
    }
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

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <div className="flex items-center justify-between px-3 py-2 border-b border-gray-700">
        <span className="text-xs font-medium text-text-secondary flex items-center gap-1.5">
          <Zap size={13} />
          Triggers
        </span>
        {!showForm && (
          <button
            onClick={() => setShowForm(true)}
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
            onCancel={() => setShowForm(false)}
            isSaving={createTrigger.isPending}
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
                onDelete={handleDelete}
                isDeleting={deleteTrigger.isPending}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
