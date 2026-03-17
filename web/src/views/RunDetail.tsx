import { useEffect, useMemo, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router';
import { ArrowLeft, XCircle, RotateCcw, Clock, Play, Wrench } from 'lucide-react';
import { toast } from 'sonner';
import { RunDAGView } from '../components/runs/RunDAGView.tsx';
import { RunStatusBadge } from '../components/runs/RunStatusBadge.tsx';
import { TerminalView } from '../components/terminal/TerminalView.tsx';
import { Breadcrumb } from '../components/shared/Breadcrumb.tsx';
import { CopyableId } from '../components/shared/CopyableId.tsx';
import { useRun, useCancelRun, useRetryTask, useRepairRun } from '../hooks/useRuns.ts';
import { useJob } from '../hooks/useJobs.ts';
import { useRunStore } from '../stores/runs.ts';
import { formatDuration } from '../lib/format.ts';

export function RunDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const { data: runDetail, isLoading: runLoading } = useRun(id);
  const run = runDetail?.run;
  const runSteps = useMemo(() => runDetail?.run_steps ?? [], [runDetail?.run_steps]);

  const { data: jobDetail, isLoading: jobLoading } = useJob(run?.job_id);
  const steps = useMemo(() => jobDetail?.steps ?? [], [jobDetail?.steps]);
  const dependencies = jobDetail?.dependencies ?? [];

  const cancelRun = useCancelRun();
  const retryTask = useRetryTask();
  const repairRun = useRepairRun();

  const selectedTaskId = useRunStore((s) => s.selectedTaskId);
  const selectTask = useRunStore((s) => s.selectTask);
  const setTaskStatuses = useRunStore((s) => s.setTaskStatuses);
  const setActiveRunId = useRunStore((s) => s.setActiveRunId);
  const taskStatuses = useRunStore((s) => s.taskStatuses);
  const reset = useRunStore((s) => s.reset);

  // Reset store when route param changes to prevent state leaking between runs
  useEffect(() => {
    setActiveRunId(id ?? null);
    return () => reset();
  }, [id, setActiveRunId, reset]);

  // Sync run tasks to store
  useEffect(() => {
    if (runSteps.length > 0) {
      setTaskStatuses(runSteps);
    }
  }, [runSteps, setTaskStatuses]);

  // Build merged run tasks from store (for live updates)
  const mergedRunTasks = useMemo(() => {
    if (taskStatuses.size === 0) return runSteps;
    return runSteps.map((rs) => taskStatuses.get(rs.step_id) ?? rs);
  }, [runSteps, taskStatuses]);

  const selectedRunTask = useMemo(
    () => mergedRunTasks.find((rs) => rs.step_id === selectedTaskId),
    [mergedRunTasks, selectedTaskId],
  );

  const selectedTaskName = useMemo(
    () => steps.find((s) => s.step_id === selectedTaskId)?.name,
    [steps, selectedTaskId],
  );

  const handleTaskSelect = useCallback(
    (taskId: string) => selectTask(taskId),
    [selectTask],
  );

  async function handleCancel() {
    if (!id) return;
    try {
      await cancelRun.mutateAsync(id);
      toast.success('Run cancelled');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to cancel run');
    }
  }

  async function handleRetry() {
    if (!id || !selectedTaskId) return;
    try {
      await retryTask.mutateAsync({ runId: id, taskId: selectedTaskId });
      toast.success('Task retrying');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to retry task');
    }
  }

  async function handleRepair() {
    if (!id) return;
    try {
      await repairRun.mutateAsync({ runId: id });
      toast.success('Run repair started');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to repair run');
    }
  }

  // Elapsed time — ticks every second for active runs
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!run?.started_at || run?.completed_at) return;
    const interval = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [run?.started_at, run?.completed_at]);

  let displayElapsed: number | null = null;
  if (run?.started_at) {
    const start = new Date(run.started_at).getTime();
    const end = run.completed_at ? new Date(run.completed_at).getTime() : now;
    displayElapsed = Math.floor((end - start) / 1000);
  }

  const isLoading = runLoading || jobLoading;

  const triggerBadge = useMemo(() => {
    const type = run?.trigger_type ?? 'manual';
    if (type === 'cron') {
      let cronExpr: string | null = null;
      if (run?.trigger_detail) {
        try {
          const detail = JSON.parse(run.trigger_detail) as { schedule_id?: string; cron_expr?: string };
          cronExpr = detail.cron_expr ?? null;
        } catch {
          // ignore parse errors
        }
      }
      return (
        <span
          className="flex items-center gap-1 text-xs px-1.5 py-0.5 rounded-full bg-blue-600/20 text-blue-400 max-w-[200px]"
          title={cronExpr ?? undefined}
        >
          <Clock size={10} />
          <span className="truncate">{cronExpr ?? 'Scheduled'}</span>
        </span>
      );
    }
    if (type === 'manual') {
      return (
        <span className="flex items-center gap-1 text-xs px-1.5 py-0.5 rounded-full bg-gray-600/20 text-gray-400">
          <Play size={10} />
          Manual
        </span>
      );
    }
    return (
      <span className="text-xs px-1.5 py-0.5 rounded-full bg-gray-600/20 text-gray-400">
        {type}
      </span>
    );
  }, [run?.trigger_type, run?.trigger_detail]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full text-text-secondary">
        Loading run...
      </div>
    );
  }

  if (!run) {
    return (
      <div className="flex items-center justify-center h-full text-text-secondary">
        Run not found
      </div>
    );
  }

  const isActive = run.status === 'running' || run.status === 'pending';
  const canRetry = selectedRunTask?.status === 'failed';
  const canRepair = run.status === 'failed' || run.status === 'cancelled';

  return (
    <div className="flex flex-col h-full">
      {/* Breadcrumb */}
      <div className="px-4 pt-2 pb-1">
        <Breadcrumb items={[
          { label: 'Runs', to: '/runs' },
          { label: `Run ${run.run_id.slice(0, 8)}` },
        ]} />
      </div>

      {/* Header */}
      <div className="flex flex-wrap items-center gap-3 px-4 py-2 bg-bg-secondary border-b border-border-primary">
        <button
          onClick={() => navigate('/runs')}
          className="text-text-secondary hover:text-text-primary transition-colors"
        >
          <ArrowLeft size={18} />
        </button>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-text-primary">Run</span>
            <CopyableId id={run.run_id} className="text-xs" />
            <RunStatusBadge status={run.status} />
            {triggerBadge}
          </div>
          {displayElapsed !== null && (
            <div className="flex items-center gap-1 text-xs text-text-secondary mt-0.5">
              <Clock size={12} />
              <span>{formatDuration(displayElapsed)}</span>
            </div>
          )}
        </div>

        {canRepair && (
          <button
            onClick={handleRepair}
            disabled={repairRun.isPending}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-orange-600/20 text-orange-400 hover:bg-orange-600/30 transition-colors disabled:opacity-40"
          >
            <Wrench size={14} />
            Repair
          </button>
        )}

        {canRetry && (
          <button
            onClick={handleRetry}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-yellow-600/20 text-yellow-400 hover:bg-yellow-600/30 transition-colors"
          >
            <RotateCcw size={14} />
            Retry Task
          </button>
        )}

        {isActive && (
          <button
            onClick={handleCancel}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-red-600/20 text-red-400 hover:bg-red-600/30 transition-colors"
          >
            <XCircle size={14} />
            Cancel Run
          </button>
        )}
      </div>

      {/* DAG View */}
      <div className="h-40 md:h-64 shrink-0 border-b border-border-primary">
        <RunDAGView
          steps={steps}
          dependencies={dependencies}
          runTasks={mergedRunTasks}
          selectedTaskId={selectedTaskId}
          onTaskSelect={handleTaskSelect}
        />
      </div>

      {/* Task Terminal */}
      <div className="flex-1 min-h-0">
        {selectedRunTask?.session_id ? (
          <div className="h-full flex flex-col">
            <div className="px-3 py-1.5 bg-bg-secondary border-b border-border-primary text-xs text-text-secondary flex items-center justify-between">
              <span className="flex items-center gap-2">
                {selectedTaskName ?? 'Task'} - {selectedRunTask.status}
                {selectedRunTask.task_type_snapshot && (
                  <span className="px-1.5 py-0.5 rounded-full bg-bg-tertiary text-text-secondary/70 text-[10px]">
                    {selectedRunTask.task_type_snapshot}
                  </span>
                )}
                <AttemptBadge attempt={selectedRunTask.attempt} />
              </span>
              {selectedRunTask.error && (
                <span className="text-red-400 truncate ml-2">{selectedRunTask.error}</span>
              )}
            </div>
            <TerminalView sessionId={selectedRunTask.session_id} className="flex-1" />
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-text-secondary text-sm">
            {selectedTaskId
              ? 'Waiting for task to start...'
              : 'Click a task in the DAG to view its terminal output'}
          </div>
        )}
      </div>
    </div>
  );
}

function AttemptBadge({ attempt }: { attempt?: number }) {
  if (!attempt || attempt <= 1) return null;
  return (
    <span className="px-1.5 py-0.5 rounded-full bg-yellow-600/20 text-yellow-400 text-[10px] font-medium">
      Attempt {attempt}
    </span>
  );
}
