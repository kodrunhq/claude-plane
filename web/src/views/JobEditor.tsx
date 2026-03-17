import { useState, useCallback, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router';
import { Plus, Save, Play, Trash2, ArrowLeft, ChevronDown, ChevronUp } from 'lucide-react';
import { JobRunHistory } from '../components/jobs/JobRunHistory.tsx';
import { toast } from 'sonner';
import { DAGCanvas } from '../components/dag/DAGCanvas.tsx';
import { TaskEditor } from '../components/jobs/TaskEditor.tsx';
import { SchedulePanel } from '../components/jobs/SchedulePanel.tsx';
import { TriggerPanel } from '../components/jobs/TriggerPanel.tsx';
import { JobMetaForm } from '../components/jobs/JobMetaForm.tsx';
import { ParameterEditor } from '../components/jobs/ParameterEditor.tsx';
import { JobSettingsPanel } from '../components/jobs/JobSettingsPanel.tsx';
import { RunNowModal } from '../components/jobs/RunNowModal.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { Breadcrumb } from '../components/shared/Breadcrumb.tsx';
import {
  useJob,
  useCreateJob,
  useUpdateJob,
  useDeleteJob,
  useAddTask,
  useUpdateTask,
  useDeleteTask,
  useAddDependency,
  useRemoveDependency,
  useTriggerRun,
} from '../hooks/useJobs.ts';
import { useMachines } from '../hooks/useMachines.ts';
import { useJobEditorStore } from '../stores/jobs.ts';
import type { UpdateTaskParams } from '../types/job.ts';

type EditorTab = 'details' | 'tasks' | 'triggers' | 'schedule' | 'settings';

const TABS: { key: EditorTab; label: string }[] = [
  { key: 'details', label: 'Details' },
  { key: 'tasks', label: 'Tasks' },
  { key: 'triggers', label: 'Triggers' },
  { key: 'schedule', label: 'Schedule' },
  { key: 'settings', label: 'Settings' },
];

export function JobEditor() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const isNew = !id || id === 'new';

  const { data: jobDetail, isLoading } = useJob(isNew ? undefined : id);
  const { data: machines } = useMachines();
  const createJob = useCreateJob();
  const updateJob = useUpdateJob();
  const deleteJobMutation = useDeleteJob();
  const addTask = useAddTask();
  const updateTask = useUpdateTask();
  const deleteTask = useDeleteTask();
  const addDependency = useAddDependency();
  const removeDependency = useRemoveDependency();
  const triggerRun = useTriggerRun();

  const selectedTaskId = useJobEditorStore((s) => s.selectedTaskId);
  const selectTask = useJobEditorStore((s) => s.selectTask);

  const [jobName, setJobName] = useState('');
  const [jobDescription, setJobDescription] = useState('');
  const [jobId, setJobId] = useState<string | null>(null);
  const [jobParams, setJobParams] = useState<Record<string, string>>({});
  const [timeoutSeconds, setTimeoutSeconds] = useState(0);
  const [maxConcurrentRuns, setMaxConcurrentRuns] = useState(1);
  const [showRunHistory, setShowRunHistory] = useState(!isNew);
  const [activeTab, setActiveTab] = useState<EditorTab>('tasks');
  const [showRunModal, setShowRunModal] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const taskDirtyRef = useRef(false);

  // Sync job data when loaded
  useEffect(() => {
    if (jobDetail) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing server data to local form state
      setJobName(jobDetail.job.name);
      setJobDescription(jobDetail.job.description);
      setJobId(jobDetail.job.job_id);
      setJobParams(jobDetail.job.parameters ?? {});
      setTimeoutSeconds(jobDetail.job.timeout_seconds ?? 0);
      setMaxConcurrentRuns(jobDetail.job.max_concurrent_runs ?? 1);
    }
  }, [jobDetail]);

  // Cleanup selection on unmount
  useEffect(() => {
    return () => selectTask(null);
  }, [selectTask]);

  const handleTaskDirtyChange = useCallback((dirty: boolean) => {
    taskDirtyRef.current = dirty;
  }, []);

  function confirmIfDirty(): boolean {
    if (!taskDirtyRef.current) return true;
    return window.confirm('You have unsaved task changes. Discard them?');
  }

  const effectiveJobId = isNew ? jobId : id;
  const steps = jobDetail?.steps ?? [];
  const dependencies = jobDetail?.dependencies ?? [];
  const selectedTask = steps.find((s) => s.step_id === selectedTaskId) ?? null;

  const handleNodeClick = useCallback(
    (taskId: string) => {
      if (taskId === selectedTaskId) return;
      if (!confirmIfDirty()) return;
      selectTask(taskId);
    },
    [selectedTaskId, selectTask],
  );

  function buildJobParams() {
    return {
      name: jobName,
      description: jobDescription,
      parameters: Object.keys(jobParams).length > 0 ? jobParams : undefined,
      timeout_seconds: timeoutSeconds || undefined,
      max_concurrent_runs: maxConcurrentRuns || undefined,
    };
  }

  async function ensureJobCreated(): Promise<string | null> {
    if (effectiveJobId) return effectiveJobId;
    if (!jobName.trim()) {
      toast.error('Enter a job name first');
      return null;
    }
    try {
      const job = await createJob.mutateAsync(buildJobParams());
      setJobId(job.job_id);
      return job.job_id;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create job');
      return null;
    }
  }

  async function handleAddTask() {
    const jid = await ensureJobCreated();
    if (!jid) return;
    if (!machines || machines.length === 0) {
      toast.error('No machines are available. Connect a machine before adding a task.');
      return;
    }
    try {
      const newTask = await addTask.mutateAsync({
        jobId: jid,
        params: {
          name: '',
          machine_id: machines[0].machine_id,
        },
      });
      selectTask(newTask.step_id);
      setActiveTab('tasks');
      if (isNew) {
        navigate(`/jobs/${jid}`, { replace: true });
      }
      toast.success('Task added');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add task');
    }
  }

  const handleConnect = useCallback(
    async (sourceTaskId: string, targetTaskId: string) => {
      if (!effectiveJobId) return;
      try {
        await addDependency.mutateAsync({
          jobId: effectiveJobId,
          taskId: targetTaskId,
          dependsOnTaskId: sourceTaskId,
        });
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Invalid dependency (possible cycle)');
      }
    },
    [effectiveJobId, addDependency],
  );

  const handleDeleteEdge = useCallback(
    async (sourceStepId: string, targetStepId: string) => {
      if (!effectiveJobId) return;
      try {
        await removeDependency.mutateAsync({
          jobId: effectiveJobId,
          taskId: targetStepId,
          depId: sourceStepId,
        });
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to remove dependency');
      }
    },
    [effectiveJobId, removeDependency],
  );

  async function handleSave() {
    if (!effectiveJobId) {
      await ensureJobCreated();
      return;
    }
    try {
      await updateJob.mutateAsync({
        id: effectiveJobId,
        params: buildJobParams(),
      });
      toast.success('Job saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save job');
    }
  }

  function handleRunClick() {
    if (!effectiveJobId) {
      toast.error('Save the job first');
      return;
    }
    if (taskDirtyRef.current) {
      toast.error('Save your task changes before running');
      return;
    }
    // If job has parameters, show the modal for overrides
    if (Object.keys(jobParams).length > 0) {
      setShowRunModal(true);
    } else {
      executeRun({});
    }
  }

  async function handleDeleteJob() {
    if (!effectiveJobId) return;
    try {
      await deleteJobMutation.mutateAsync(effectiveJobId);
      setShowDeleteConfirm(false);
      toast.success('Job deleted');
      navigate('/jobs');
    } catch (err) {
      setShowDeleteConfirm(false);
      toast.error(err instanceof Error ? err.message : 'Failed to delete job');
    }
  }

  async function executeRun(parameters: Record<string, string>) {
    if (!effectiveJobId) return;
    setShowRunModal(false);
    try {
      const hasParams = Object.keys(parameters).length > 0;
      const run = await triggerRun.mutateAsync({
        jobId: effectiveJobId,
        params: hasParams ? { parameters } : undefined,
      });
      toast.success('Run started');
      navigate(`/runs/${run.run_id}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start run');
    }
  }

  function handleTaskSave(taskId: string, params: UpdateTaskParams) {
    if (!effectiveJobId) return;
    updateTask.mutate(
      { jobId: effectiveJobId, taskId, params },
      {
        onSuccess: () => toast.success('Task updated'),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to update task'),
      },
    );
  }

  function handleTaskDelete(taskId: string) {
    if (!effectiveJobId) return;
    deleteTask.mutate(
      { jobId: effectiveJobId, taskId },
      {
        onSuccess: () => {
          selectTask(null);
          toast.success('Task deleted');
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete task'),
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
      {/* Breadcrumb */}
      <div className="px-4 pt-2 pb-1">
        <Breadcrumb items={[
          { label: 'Jobs', to: '/jobs' },
          { label: jobName || (isNew ? 'New Job' : 'Job') },
        ]} />
      </div>

      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-3 px-4 py-2 bg-bg-secondary border-b border-border-primary">
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
          onClick={handleAddTask}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
        >
          <Plus size={14} />
          Add Task
        </button>
        <button
          onClick={handleSave}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Save size={14} />
          Save
        </button>
        <button
          onClick={handleRunClick}
          disabled={!effectiveJobId || steps.length === 0}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-green-600 hover:bg-green-600/80 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          <Play size={14} />
          Run
        </button>
        {!isNew && (
          <button
            type="button"
            aria-label="Delete job"
            onClick={() => setShowDeleteConfirm(true)}
            className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
            title="Delete job"
          >
            <Trash2 size={16} />
          </button>
        )}
      </div>

      {/* Tab bar */}
      <div className="flex border-b border-border-primary bg-bg-secondary px-4 shrink-0 overflow-x-auto">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => {
              if (!confirmIfDirty()) return;
              setActiveTab(tab.key);
            }}
            className={`px-4 py-2 text-xs font-medium transition-colors ${
              activeTab === tab.key
                ? 'text-text-primary border-b-2 border-accent-primary'
                : 'text-text-secondary hover:text-text-primary'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Main content */}
      <div className="flex flex-col flex-1 min-h-0">
        {/* Details tab */}
        {activeTab === 'details' && (
          <div className="flex-1 overflow-y-auto p-6 max-w-2xl space-y-6">
            <JobMetaForm
              name={jobName}
              description={jobDescription}
              onChange={handleMetaChange}
            />
            <ParameterEditor
              parameters={jobParams}
              onChange={setJobParams}
            />
          </div>
        )}

        {/* Tasks tab — preserves the original DAG + TaskEditor layout */}
        {activeTab === 'tasks' && (
          <div className="flex flex-col md:flex-row flex-1 min-h-0">
            {/* DAG Canvas (left on desktop, top on mobile) */}
            <div className="flex-1 min-w-0 min-h-[200px] md:min-h-0">
              {steps.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-text-secondary text-sm gap-2">
                  <p>No tasks yet. Click "Add Task" to begin.</p>
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
                  selectedTaskId={selectedTaskId}
                  onNodeClick={handleNodeClick}
                  onConnect={handleConnect}
                  onDeleteEdge={handleDeleteEdge}
                />
              )}
            </div>

            {/* Task Editor sidebar (right on desktop, below on mobile) */}
            <div className="w-full md:w-80 border-t md:border-t-0 md:border-l border-border-primary bg-bg-secondary shrink-0 flex flex-col max-h-[50vh] md:max-h-none overflow-y-auto">
              <TaskEditor
                task={selectedTask}
                machines={machines ?? []}
                onSave={handleTaskSave}
                onDelete={handleTaskDelete}
                onDirtyChange={handleTaskDirtyChange}
              />
            </div>
          </div>
        )}

        {/* Triggers tab — full width */}
        {activeTab === 'triggers' && (
          <div className="flex-1 overflow-y-auto">
            {effectiveJobId ? (
              <TriggerPanel jobId={effectiveJobId} />
            ) : (
              <div className="flex items-center justify-center h-full text-text-secondary text-sm">
                Save the job first to configure triggers
              </div>
            )}
          </div>
        )}

        {/* Schedule tab — full width */}
        {activeTab === 'schedule' && (
          <div className="flex-1 overflow-y-auto">
            {effectiveJobId ? (
              <SchedulePanel jobId={effectiveJobId} />
            ) : (
              <div className="flex items-center justify-center h-full text-text-secondary text-sm">
                Save the job first to configure schedules
              </div>
            )}
          </div>
        )}

        {/* Settings tab */}
        {activeTab === 'settings' && (
          <div className="flex-1 overflow-y-auto">
            <JobSettingsPanel
              timeoutSeconds={timeoutSeconds}
              maxConcurrentRuns={maxConcurrentRuns}
              onTimeoutChange={setTimeoutSeconds}
              onMaxConcurrentChange={setMaxConcurrentRuns}
            />
          </div>
        )}

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

      {/* Run Now Modal */}
      {showRunModal && (
        <RunNowModal
          defaultParameters={jobParams}
          steps={steps}
          onRun={executeRun}
          onClose={() => setShowRunModal(false)}
        />
      )}

      <ConfirmDialog
        open={showDeleteConfirm}
        title="Delete Job"
        message={`Are you sure you want to delete "${jobName}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDeleteJob}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
  );
}
