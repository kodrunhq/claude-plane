import { useNavigate } from 'react-router';
import { Plus, Play, AlertCircle, RefreshCw } from 'lucide-react';
import { useJobs, useTriggerRun } from '../hooks/useJobs.ts';
import { formatTimeAgo, truncateId } from '../lib/format.ts';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { toast } from 'sonner';

const statusColors: Record<string, string> = {
  pending: 'text-gray-400',
  running: 'text-blue-400',
  completed: 'text-green-400',
  failed: 'text-red-400',
  cancelled: 'text-yellow-400',
};

export function JobsPage() {
  const navigate = useNavigate();
  const { data: jobs, isLoading, error, refetch } = useJobs();
  const triggerRun = useTriggerRun();

  async function handleRun(e: React.MouseEvent, jobId: string) {
    e.stopPropagation();
    try {
      const run = await triggerRun.mutateAsync(jobId);
      toast.success('Run started');
      navigate(`/runs/${run.run_id}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start run');
    }
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load jobs'}
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
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Jobs</h1>
        <button
          onClick={() => navigate('/jobs/new')}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Plus size={16} />
          New Job
        </button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-bg-secondary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-tertiary rounded w-1/4 mb-2" />
              <div className="h-3 bg-bg-tertiary rounded w-1/2" />
            </div>
          ))}
        </div>
      ) : !jobs || jobs.length === 0 ? (
        <EmptyState title="No jobs yet" description="Create your first job to get started." />
      ) : (
        <div className="space-y-2">
          {jobs.map((job) => (
            <div
              key={job.job_id}
              onClick={() => navigate(`/jobs/${job.job_id}`)}
              className="bg-bg-secondary rounded-lg p-4 flex items-center gap-4 cursor-pointer hover:bg-bg-tertiary/50 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-text-primary truncate">
                    {job.name}
                  </span>
                  <span className="text-xs text-text-secondary">
                    {truncateId(job.job_id)}
                  </span>
                </div>
                <div className="flex items-center gap-3 mt-1 text-xs text-text-secondary">
                  <span>{job.step_count ?? 0} steps</span>
                  {job.last_run_status && (
                    <span className={statusColors[job.last_run_status] ?? 'text-gray-400'}>
                      {job.last_run_status}
                    </span>
                  )}
                  <span>{formatTimeAgo(job.created_at)}</span>
                </div>
              </div>
              <button
                onClick={(e) => handleRun(e, job.job_id)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-green-600/20 text-green-400 hover:bg-green-600/30 transition-colors shrink-0"
                title="Run job"
              >
                <Play size={14} />
                Run
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
