import { useState } from 'react';
import { Link } from 'react-router';
import { Zap, Trash2, ToggleLeft, ToggleRight, AlertCircle, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { useAllTriggers, useToggleTrigger, useDeleteTrigger } from '../hooks/useTriggers.ts';
import { RefreshButton } from '../components/shared/RefreshButton.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { TimeAgo } from '../components/shared/TimeAgo.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import type { JobTriggerWithJob } from '../types/trigger.ts';

export function TriggersPage() {
  const { data: triggers, isLoading, error, refetch, isFetching } = useAllTriggers();
  const toggleTrigger = useToggleTrigger();
  const deleteTrigger = useDeleteTrigger();

  const [deleteTarget, setDeleteTarget] = useState<JobTriggerWithJob | null>(null);

  function handleToggle(trigger: JobTriggerWithJob) {
    toggleTrigger.mutate(
      { triggerId: trigger.trigger_id },
      {
        onSuccess: (updated) => {
          toast.success(`Trigger ${updated.enabled ? 'enabled' : 'disabled'}`);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : 'Failed to toggle trigger');
        },
      },
    );
  }

  function handleDeleteConfirm() {
    if (!deleteTarget) return;
    deleteTrigger.mutate(
      { triggerId: deleteTarget.trigger_id, jobId: deleteTarget.job_id },
      {
        onSuccess: () => {
          toast.success('Trigger deleted');
          setDeleteTarget(null);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : 'Failed to delete trigger');
          setDeleteTarget(null);
        },
      },
    );
  }

  if (error) {
    return (
      <div className="p-4 md:p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load triggers'}
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
    <div className="p-4 md:p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Triggers</h1>
        <RefreshButton onClick={() => refetch()} loading={isFetching} />
      </div>

      {isLoading ? (
        <SkeletonTable rows={4} columns={6} />
      ) : !triggers || triggers.length === 0 ? (
        <EmptyState
          icon={<Zap size={40} />}
          title="No triggers yet"
          description="Triggers automatically start job runs when matching events occur. Create triggers from a job's detail page."
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-border-primary">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-bg-secondary text-text-secondary text-left">
                <th className="px-4 py-3 font-medium">Event Type</th>
                <th className="px-4 py-3 font-medium">Filter</th>
                <th className="px-4 py-3 font-medium">Job</th>
                <th className="px-4 py-3 font-medium">Enabled</th>
                <th className="px-4 py-3 font-medium">Created</th>
                <th className="px-4 py-3 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border-primary">
              {triggers.map((trigger) => (
                <tr key={trigger.trigger_id} className="hover:bg-bg-tertiary/40 transition-colors">
                  <td className="px-4 py-3">
                    <code className="text-xs bg-bg-tertiary px-1.5 py-0.5 rounded font-mono">
                      {trigger.event_type}
                    </code>
                  </td>
                  <td className="px-4 py-3 text-text-secondary max-w-[200px] truncate">
                    {trigger.filter || <span className="text-text-secondary/50">none</span>}
                  </td>
                  <td className="px-4 py-3">
                    <Link
                      to={`/jobs/${trigger.job_id}`}
                      className="text-accent-primary hover:underline"
                    >
                      {trigger.job_name}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    {trigger.enabled ? (
                      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-status-success">
                        <span className="w-1.5 h-1.5 rounded-full bg-status-success" />
                        Enabled
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-text-secondary">
                        <span className="w-1.5 h-1.5 rounded-full bg-text-secondary/40" />
                        Disabled
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-text-secondary">
                    <TimeAgo date={trigger.created_at} />
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        onClick={() => handleToggle(trigger)}
                        className="p-1.5 rounded text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
                        title={trigger.enabled ? 'Disable trigger' : 'Enable trigger'}
                      >
                        {trigger.enabled ? <ToggleRight size={16} /> : <ToggleLeft size={16} />}
                      </button>
                      <button
                        onClick={() => setDeleteTarget(trigger)}
                        className="p-1.5 rounded text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
                        title="Delete trigger"
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Trigger"
        message={`Delete the "${deleteTarget?.event_type}" trigger for job "${deleteTarget?.job_name}"? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
