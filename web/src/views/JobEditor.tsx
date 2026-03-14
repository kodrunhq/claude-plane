import { useState, useCallback, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router';
import { Plus, Save, Play, ArrowLeft, ChevronDown, ChevronUp } from 'lucide-react';
import { JobRunHistory } from '../components/jobs/JobRunHistory.tsx';
import { toast } from 'sonner';
import { DAGCanvas } from '../components/dag/DAGCanvas.tsx';
import { StepEditor } from '../components/jobs/StepEditor.tsx';
import { SchedulePanel } from '../components/jobs/SchedulePanel.tsx';
import { TriggerPanel } from '../components/jobs/TriggerPanel.tsx';
import { JobMetaForm } from '../components/jobs/JobMetaForm.tsx';
import {
  useJob,
  useCreateJob,
  useUpdateJob,
  useAddStep,
  useUpdateStep,
  useDeleteStep,
  useAddDependency,
  useTriggerRun,
} from '../hooks/useJobs.ts';
import { useMachines } from '../hooks/useMachines.ts';
import { useJobEditorStore } from '../stores/jobs.ts';
import type { UpdateStepParams } from '../types/job.ts';

export function JobEditor() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const isNew = !id || id === 'new';

  const { data: jobDetail, isLoading } = useJob(isNew ? undefined : id);
  const { data: machines } = useMachines();
  const createJob = useCreateJob();
  const updateJob = useUpdateJob();
  const addStep = useAddStep();
  const updateStep = useUpdateStep();
  const deleteStep = useDeleteStep();
  const addDependency = useAddDependency();
  const triggerRun = useTriggerRun();

  const selectedStepId = useJobEditorStore((s) => s.selectedStepId);
  const selectStep = useJobEditorStore((s) => s.selectStep);

  const [jobName, setJobName] = useState('');
  const [jobDescription, setJobDescription] = useState('');
  const [jobId, setJobId] = useState<string | null>(null);
  const [showRunHistory, setShowRunHistory] = useState(!isNew);
  const [rightTab, setRightTab] = useState<'steps' | 'schedules' | 'triggers'>('steps');
  const stepDirtyRef = useRef(false);

  // Sync job data when loaded
  useEffect(() => {
    if (jobDetail) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing server data to local form state
      setJobName(jobDetail.job.name);
      setJobDescription(jobDetail.job.description);
      setJobId(jobDetail.job.job_id);
    }
  }, [jobDetail]);

  // Cleanup selection on unmount
  useEffect(() => {
    return () => selectStep(null);
  }, [selectStep]);

  const handleStepDirtyChange = useCallback((dirty: boolean) => {
    stepDirtyRef.current = dirty;
  }, []);

  function confirmIfDirty(): boolean {
    if (!stepDirtyRef.current) return true;
    return window.confirm('You have unsaved step changes. Discard them?');
  }

  const effectiveJobId = isNew ? jobId : id;
  const steps = jobDetail?.steps ?? [];
  const dependencies = jobDetail?.dependencies ?? [];
  const selectedStep = steps.find((s) => s.step_id === selectedStepId) ?? null;

  const handleNodeClick = useCallback(
    (stepId: string) => {
      if (stepId === selectedStepId) return;
      if (!confirmIfDirty()) return;
      selectStep(stepId);
    },
    [selectedStepId, selectStep],
  );

  async function ensureJobCreated(): Promise<string | null> {
    if (effectiveJobId) return effectiveJobId;
    if (!jobName.trim()) {
      toast.error('Enter a job name first');
      return null;
    }
    try {
      const job = await createJob.mutateAsync({ name: jobName, description: jobDescription });
      setJobId(job.job_id);
      navigate(`/jobs/${job.job_id}`, { replace: true });
      return job.job_id;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create job');
      return null;
    }
  }

  async function handleAddStep() {
    const jid = await ensureJobCreated();
    if (!jid) return;
    try {
      const step = await addStep.mutateAsync({
        jobId: jid,
        params: {
          name: `Step ${steps.length + 1}`,
          machine_id: machines?.[0]?.machine_id ?? '',
        },
      });
      selectStep(step.step_id);
      toast.success('Step added');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add step');
    }
  }

  const handleConnect = useCallback(
    async (sourceStepId: string, targetStepId: string) => {
      if (!effectiveJobId) return;
      try {
        await addDependency.mutateAsync({
          jobId: effectiveJobId,
          stepId: targetStepId,
          dependsOnStepId: sourceStepId,
        });
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Invalid dependency (possible cycle)');
      }
    },
    [effectiveJobId, addDependency],
  );

  async function handleSave() {
    if (!effectiveJobId) {
      await ensureJobCreated();
      return;
    }
    try {
      await updateJob.mutateAsync({
        id: effectiveJobId,
        params: { name: jobName, description: jobDescription },
      });
      toast.success('Job saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save job');
    }
  }

  async function handleRun() {
    if (!effectiveJobId) {
      toast.error('Save the job first');
      return;
    }
    if (stepDirtyRef.current) {
      toast.error('Save your step changes before running');
      return;
    }
    try {
      const run = await triggerRun.mutateAsync(effectiveJobId);
      toast.success('Run started');
      navigate(`/runs/${run.run_id}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start run');
    }
  }

  function handleStepSave(stepId: string, params: UpdateStepParams) {
    if (!effectiveJobId) return;
    updateStep.mutate(
      { jobId: effectiveJobId, stepId, params },
      {
        onSuccess: () => toast.success('Step updated'),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to update step'),
      },
    );
  }

  function handleStepDelete(stepId: string) {
    if (!effectiveJobId) return;
    deleteStep.mutate(
      { jobId: effectiveJobId, stepId },
      {
        onSuccess: () => {
          selectStep(null);
          toast.success('Step deleted');
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete step'),
      },
    );
  }

  function handleMetaChange(field: 'name' | 'description', value: string) {
    if (field === 'name') setJobName(value);
    else setJobDescription(value);
  }

  if (!isNew && isLoading) {
    return (
      <div className="flex items-center justify-center h-full text-text-secondary">
        Loading job...
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-3 px-4 py-2 bg-bg-secondary border-b border-border-primary">
        <button
          onClick={() => navigate('/jobs')}
          className="text-text-secondary hover:text-text-primary transition-colors"
          title="Back to jobs"
        >
          <ArrowLeft size={18} />
        </button>

        <input
          type="text"
          value={jobName}
          onChange={(e) => setJobName(e.target.value)}
          className="bg-transparent text-text-primary text-sm font-medium focus:outline-none border-b border-transparent focus:border-accent-primary flex-1 min-w-0"
          placeholder="Job name..."
        />

        <button
          onClick={handleAddStep}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
        >
          <Plus size={14} />
          Add Step
        </button>
        <button
          onClick={handleSave}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Save size={14} />
          Save
        </button>
        <button
          onClick={handleRun}
          disabled={!effectiveJobId || steps.length === 0}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-green-600 hover:bg-green-600/80 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Play size={14} />
          Run
        </button>
      </div>

      {/* Main content */}
      <div className="flex flex-col flex-1 min-h-0">
        {/* DAG + Step Editor row */}
        <div className="flex flex-1 min-h-0">
          {/* DAG Canvas (left) */}
          <div className="flex-1 min-w-0">
            {steps.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-text-secondary text-sm gap-2">
                <p>No steps yet. Click "Add Step" to begin.</p>
                {isNew && !jobId && (
                  <div className="mt-4 w-64">
                    <JobMetaForm
                      name={jobName}
                      description={jobDescription}
                      onChange={handleMetaChange}
                    />
                  </div>
                )}
              </div>
            ) : (
              <DAGCanvas
                steps={steps}
                dependencies={dependencies}
                editable
                selectedStepId={selectedStepId}
                onNodeClick={handleNodeClick}
                onConnect={handleConnect}
              />
            )}
          </div>

          {/* Right panel */}
          <div className="w-80 border-l border-border-primary bg-bg-secondary shrink-0 flex flex-col">
            {effectiveJobId ? (
              <>
                {/* Tab bar */}
                <div className="flex border-b border-border-primary shrink-0">
                  <button
                    onClick={() => setRightTab('steps')}
                    className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                      rightTab === 'steps'
                        ? 'text-text-primary border-b-2 border-accent-primary'
                        : 'text-text-secondary hover:text-text-primary'
                    }`}
                  >
                    Steps
                  </button>
                  <button
                    onClick={() => setRightTab('schedules')}
                    className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                      rightTab === 'schedules'
                        ? 'text-text-primary border-b-2 border-accent-primary'
                        : 'text-text-secondary hover:text-text-primary'
                    }`}
                  >
                    Schedules
                  </button>
                  <button
                    onClick={() => setRightTab('triggers')}
                    className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                      rightTab === 'triggers'
                        ? 'text-text-primary border-b-2 border-accent-primary'
                        : 'text-text-secondary hover:text-text-primary'
                    }`}
                  >
                    Triggers
                  </button>
                </div>
                {rightTab === 'steps' && (
                  <StepEditor
                    step={selectedStep}
                    machines={machines ?? []}
                    onSave={handleStepSave}
                    onDelete={handleStepDelete}
                    onDirtyChange={handleStepDirtyChange}
                  />
                )}
                {rightTab === 'schedules' && <SchedulePanel jobId={effectiveJobId} />}
                {rightTab === 'triggers' && <TriggerPanel jobId={effectiveJobId} />}
              </>
            ) : (
              <StepEditor
                step={selectedStep}
                machines={machines ?? []}
                onSave={handleStepSave}
                onDelete={handleStepDelete}
              />
            )}
          </div>
        </div>

        {/* Run History (collapsible, only for existing jobs) */}
        {effectiveJobId && (
          <div className="border-t border-border-primary shrink-0">
            <button
              onClick={() => setShowRunHistory((prev) => !prev)}
              className="flex items-center gap-2 w-full px-4 py-2 text-xs font-medium text-text-secondary hover:text-text-primary bg-bg-secondary hover:bg-bg-tertiary transition-colors"
            >
              <span className="flex-1 text-left">Run History</span>
              {showRunHistory ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            </button>
            {showRunHistory && (
              <div className="max-h-64 overflow-y-auto">
                <JobRunHistory
                  jobId={effectiveJobId}
                  onRunClick={(runId) => navigate(`/runs/${runId}`)}
                />
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
