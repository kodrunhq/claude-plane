import { useState } from 'react';
import { Link } from 'react-router';
import { Clock, Trash2, Pause, Play, PlayCircle, AlertCircle, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import cronstrue from 'cronstrue';
import { useAllSchedules, usePauseSchedule, useResumeSchedule, useDeleteSchedule, useTriggerSchedule } from '../hooks/useSchedules.ts';
import { RefreshButton } from '../components/shared/RefreshButton.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { TimeAgo } from '../components/shared/TimeAgo.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import type { CronScheduleWithJob } from '../types/schedule.ts';

function formatCronDescription(expr: string): string {
  try {
    return cronstrue.toString(expr, { throwExceptionOnParseError: true });
  } catch {
    return expr;
  }
}

export function SchedulesPage() {
  const { data: schedules, isLoading, error, refetch, isFetching } = useAllSchedules();
  const pauseSchedule = usePauseSchedule();
  const resumeSchedule = useResumeSchedule();
  const deleteSchedule = useDeleteSchedule();
  const triggerSchedule = useTriggerSchedule();

  const [deleteTarget, setDeleteTarget] = useState<CronScheduleWithJob | null>(null);

  function handleTrigger(schedule: CronScheduleWithJob) {
    triggerSchedule.mutate(schedule.schedule_id, {
      onSuccess: () => {
        toast.success('Run triggered');
      },
      onError: (err) => {
        toast.error(err instanceof Error ? err.message : 'Failed to trigger run');
      },
    });
  }

  function handleToggleEnabled(schedule: CronScheduleWithJob) {
    const mutation = schedule.enabled ? pauseSchedule : resumeSchedule;
    mutation.mutate(schedule.schedule_id, {
      onSuccess: (updated) => {
        toast.success(`Schedule ${updated.enabled ? 'resumed' : 'paused'}`);
      },
      onError: (err) => {
        toast.error(err instanceof Error ? err.message : 'Failed to update schedule');
      },
    });
  }

  function handleDeleteConfirm() {
    if (!deleteTarget) return;
    deleteSchedule.mutate(
      { id: deleteTarget.schedule_id, jobId: deleteTarget.job_id },
      {
        onSuccess: () => {
          toast.success('Schedule deleted');
          setDeleteTarget(null);
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : 'Failed to delete schedule');
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
            {error instanceof Error ? error.message : 'Failed to load schedules'}
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
        <h1 className="text-xl font-semibold text-text-primary">Schedules</h1>
        <RefreshButton onClick={() => refetch()} loading={isFetching} />
      </div>

      {isLoading ? (
        <SkeletonTable rows={4} columns={6} />
      ) : !schedules || schedules.length === 0 ? (
        <EmptyState
          icon={<Clock size={40} />}
          title="No schedules yet"
          description="Schedules run jobs on a cron-based cadence. Create schedules from a job's detail page."
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-border-primary">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-bg-secondary text-text-secondary text-left">
                <th className="px-4 py-3 font-medium">Cron</th>
                <th className="px-4 py-3 font-medium">Timezone</th>
                <th className="px-4 py-3 font-medium">Job</th>
                <th className="px-4 py-3 font-medium">Next Run</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border-primary">
              {schedules.map((schedule) => (
                <tr key={schedule.schedule_id} className="hover:bg-bg-tertiary/40 transition-colors">
                  <td className="px-4 py-3">
                    <div className="flex flex-col gap-0.5">
                      <code className="text-xs bg-bg-tertiary px-1.5 py-0.5 rounded font-mono w-fit">
                        {schedule.cron_expr}
                      </code>
                      <span className="text-xs text-text-secondary">
                        {formatCronDescription(schedule.cron_expr)}
                      </span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-text-secondary">
                    {schedule.timezone}
                  </td>
                  <td className="px-4 py-3">
                    <Link
                      to={`/jobs/${schedule.job_id}`}
                      className="text-accent-primary hover:underline"
                    >
                      {schedule.job_name}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-text-secondary">
                    {schedule.next_run_at ? (
                      <TimeAgo date={schedule.next_run_at} />
                    ) : (
                      <span className="text-text-secondary/50">--</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    {schedule.enabled ? (
                      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-status-success">
                        <span className="w-1.5 h-1.5 rounded-full bg-status-success" />
                        Enabled
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-status-warning">
                        <span className="w-1.5 h-1.5 rounded-full bg-status-warning" />
                        Paused
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        onClick={() => handleTrigger(schedule)}
                        disabled={triggerSchedule.isPending}
                        className="p-1.5 rounded text-text-secondary hover:text-accent-primary hover:bg-bg-tertiary transition-colors disabled:opacity-50"
                        title="Run now"
                        aria-label="Run now"
                      >
                        <PlayCircle size={16} />
                      </button>
                      <button
                        onClick={() => handleToggleEnabled(schedule)}
                        className="p-1.5 rounded text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
                        title={schedule.enabled ? 'Pause schedule' : 'Resume schedule'}
                        aria-label={schedule.enabled ? 'Pause schedule' : 'Resume schedule'}
                      >
                        {schedule.enabled ? <Pause size={16} /> : <Play size={16} />}
                      </button>
                      <button
                        onClick={() => setDeleteTarget(schedule)}
                        className="p-1.5 rounded text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
                        title="Delete schedule"
                        aria-label="Delete schedule"
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
        title="Delete Schedule"
        message={`Delete the "${deleteTarget?.cron_expr}" schedule for job "${deleteTarget?.job_name}"? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
