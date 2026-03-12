import { useEffect, useMemo, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router';
import { ArrowLeft, XCircle, RotateCcw, Clock } from 'lucide-react';
import { toast } from 'sonner';
import { RunDAGView } from '../components/runs/RunDAGView.tsx';
import { TerminalView } from '../components/terminal/TerminalView.tsx';
import { useRun, useCancelRun, useRetryStep } from '../hooks/useRuns.ts';
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
  const retryStep = useRetryStep();

  const selectedStepId = useRunStore((s) => s.selectedStepId);
  const selectStep = useRunStore((s) => s.selectStep);
  const setStepStatuses = useRunStore((s) => s.setStepStatuses);
  const setActiveRunId = useRunStore((s) => s.setActiveRunId);
  const stepStatuses = useRunStore((s) => s.stepStatuses);
  const reset = useRunStore((s) => s.reset);

  // Reset store when route param changes to prevent state leaking between runs
  useEffect(() => {
    setActiveRunId(id ?? null);
    return () => reset();
  }, [id, setActiveRunId, reset]);

  // Sync run steps to store
  useEffect(() => {
    if (runSteps.length > 0) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing server data to Zustand store
      setStepStatuses(runSteps);
    }
  }, [runSteps, setStepStatuses]);

  // Build merged run steps from store (for live updates)
  const mergedRunSteps = useMemo(() => {
    if (stepStatuses.size === 0) return runSteps;
    return runSteps.map((rs) => stepStatuses.get(rs.step_id) ?? rs);
  }, [runSteps, stepStatuses]);

  const selectedRunStep = useMemo(
    () => mergedRunSteps.find((rs) => rs.step_id === selectedStepId),
    [mergedRunSteps, selectedStepId],
  );

  const selectedStepName = useMemo(
    () => steps.find((s) => s.step_id === selectedStepId)?.name,
    [steps, selectedStepId],
  );

  const handleStepSelect = useCallback(
    (stepId: string) => selectStep(stepId),
    [selectStep],
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
    if (!id || !selectedStepId) return;
    try {
      await retryStep.mutateAsync({ runId: id, stepId: selectedStepId });
      toast.success('Step retrying');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to retry step');
    }
  }

  // Elapsed time — ticks every second for active runs
  const [elapsed, setElapsed] = useState<number | null>(null);
  useEffect(() => {
    if (!run?.started_at) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing derived state from run timestamps
      setElapsed(null);
      return;
    }
    const start = new Date(run.started_at).getTime();
    if (run.completed_at) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- computing final elapsed from completed run
      setElapsed(Math.floor((new Date(run.completed_at).getTime() - start) / 1000));
      return;
    }
    const update = () => setElapsed(Math.floor((Date.now() - start) / 1000));
    update();
    const interval = setInterval(update, 1000);
    return () => clearInterval(interval);
  }, [run?.started_at, run?.completed_at]);

  const isLoading = runLoading || jobLoading;

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
  const canRetry = selectedRunStep?.status === 'failed';

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2 bg-bg-secondary border-b border-gray-700">
        <button
          onClick={() => navigate('/jobs')}
          className="text-text-secondary hover:text-text-primary transition-colors"
        >
          <ArrowLeft size={18} />
        </button>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-text-primary">
              Run {id?.slice(0, 8)}
            </span>
            <RunStatusBadge status={run.status} />
          </div>
          {elapsed !== null && (
            <div className="flex items-center gap-1 text-xs text-text-secondary mt-0.5">
              <Clock size={12} />
              <span>{formatDuration(elapsed)}</span>
            </div>
          )}
        </div>

        {canRetry && (
          <button
            onClick={handleRetry}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-yellow-600/20 text-yellow-400 hover:bg-yellow-600/30 transition-colors"
          >
            <RotateCcw size={14} />
            Retry Step
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
      <div className="h-64 shrink-0 border-b border-gray-700">
        <RunDAGView
          steps={steps}
          dependencies={dependencies}
          runSteps={mergedRunSteps}
          selectedStepId={selectedStepId}
          onStepSelect={handleStepSelect}
        />
      </div>

      {/* Step Terminal */}
      <div className="flex-1 min-h-0">
        {selectedRunStep?.session_id ? (
          <div className="h-full flex flex-col">
            <div className="px-3 py-1.5 bg-bg-secondary border-b border-gray-700 text-xs text-text-secondary flex items-center justify-between">
              <span>
                {selectedStepName ?? 'Step'} - {selectedRunStep.status}
              </span>
              {selectedRunStep.error && (
                <span className="text-red-400 truncate ml-2">{selectedRunStep.error}</span>
              )}
            </div>
            <TerminalView sessionId={selectedRunStep.session_id} className="flex-1" />
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-text-secondary text-sm">
            {selectedStepId
              ? 'Waiting for step to start...'
              : 'Click a step in the DAG to view its terminal output'}
          </div>
        )}
      </div>
    </div>
  );
}

function RunStatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    pending: 'bg-gray-500/20 text-gray-400',
    running: 'bg-blue-500/20 text-blue-400',
    completed: 'bg-green-500/20 text-green-400',
    failed: 'bg-red-500/20 text-red-400',
    cancelled: 'bg-yellow-500/20 text-yellow-400',
  };

  return (
    <span className={`px-2 py-0.5 rounded-full text-xs ${colors[status] ?? colors.pending}`}>
      {status}
    </span>
  );
}
