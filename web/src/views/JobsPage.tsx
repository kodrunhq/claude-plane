import { useState, useMemo, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Plus, Play, Trash2, AlertCircle, RefreshCw, Search, CopyPlus } from 'lucide-react';
import { useJobs, useDeleteJob, useTriggerRun, useCloneJob } from '../hooks/useJobs.ts';
import { useSortableTable } from '../hooks/useSortableTable.ts';
import { usePagination } from '../hooks/usePagination.ts';
import { formatTimeAgo } from '../lib/format.ts';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { RefreshButton } from '../components/shared/RefreshButton.tsx';
import { CopyableId } from '../components/shared/CopyableId.tsx';
import { SortableHeader } from '../components/shared/SortableHeader.tsx';
import { Pagination } from '../components/shared/Pagination.tsx';
import { RunStatusBadge } from '../components/runs/RunStatusBadge.tsx';
import { toast } from 'sonner';
import type { Job } from '../types/job.ts';

const STATUS_OPTIONS = [
  { value: 'all', label: 'All Statuses' },
  { value: 'pending', label: 'Pending' },
  { value: 'running', label: 'Running' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
  { value: 'cancelled', label: 'Cancelled' },
];

const SELECT_CLASS =
  'rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary';

function TriggerBadge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    manual: 'bg-gray-500/20 text-gray-400',
    cron: 'bg-blue-500/20 text-blue-400',
    event: 'bg-purple-500/20 text-purple-400',
    mixed: 'bg-amber-500/20 text-amber-400',
  };
  return (
    <span className={`inline-flex px-2 py-0.5 text-xs rounded-full ${styles[type] ?? styles.manual}`}>
      {type}
    </span>
  );
}

function formatMachineIds(ids: string | undefined): string {
  if (!ids) return '—';
  const machines = ids.split(',');
  if (machines.length === 1) return machines[0].slice(0, 12);
  return `${machines[0].slice(0, 12)} +${machines.length - 1}`;
}

export function JobsPage() {
  const navigate = useNavigate();
  const { data: jobs, isLoading, isFetching, error, refetch } = useJobs();
  const triggerRun = useTriggerRun();
  const cloneJob = useCloneJob();
  const deleteJob = useDeleteJob();

  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [deleteTarget, setDeleteTarget] = useState<Job | null>(null);

  const filteredJobs = useMemo(() => {
    if (!jobs) return [];
    return jobs.filter((job: Job) => {
      const matchesSearch =
        searchQuery === '' ||
        job.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        job.job_id.toLowerCase().includes(searchQuery.toLowerCase());
      const matchesStatus =
        statusFilter === 'all' || job.last_run_status === statusFilter;
      return matchesSearch && matchesStatus;
    });
  }, [jobs, searchQuery, statusFilter]);

  const { sorted: sortedJobs, sort, dir, handleSort } = useSortableTable(filteredJobs, 'created_at', 'desc');

  const { paged: pagedJobs, page, pageSize, total, setPage, setPageSize } = usePagination(sortedJobs);

  useEffect(() => { setPage(1); }, [searchQuery, statusFilter, sort, dir, setPage]);

  async function handleRun(e: React.MouseEvent, jobId: string) {
    e.stopPropagation();
    try {
      const run = await triggerRun.mutateAsync({ jobId });
      toast.success('Run started');
      navigate(`/runs/${run.run_id}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start run');
    }
  }

  async function handleClone(e: React.MouseEvent, job: Job) {
    e.stopPropagation();
    try {
      const cloned = await cloneJob.mutateAsync({ id: job.job_id });
      toast.success('Job duplicated');
      navigate(`/jobs/${cloned.job.job_id}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to duplicate job');
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    try {
      await deleteJob.mutateAsync(deleteTarget.job_id);
      toast.success('Job deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete job');
    }
    setDeleteTarget(null);
  }

  if (error) {
    return (
      <div className="p-4 md:p-6">
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
    <div className="p-4 md:p-6 space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Jobs</h1>
        <div className="flex items-center gap-2">
          <RefreshButton onClick={() => refetch()} loading={isFetching} />
          <button
            onClick={() => navigate('/jobs/new')}
            className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
          >
            <Plus size={16} />
            New Job
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-4">
        <div className="relative flex-1 max-w-xs">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
          <input
            type="text"
            placeholder="Search by name or ID..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            aria-label="Search jobs"
            className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm pl-9 pr-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/50"
          />
        </div>
        <div>
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className={SELECT_CLASS}>
            {STATUS_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
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
      ) : sortedJobs.length === 0 ? (
        <EmptyState
          title={jobs && jobs.length > 0 ? 'No matching jobs' : 'No jobs yet'}
          description={jobs && jobs.length > 0 ? 'Try adjusting your search or filters.' : 'Create your first job to get started.'}
        />
      ) : (
        <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left border-b border-border-primary">
              <SortableHeader label="Name" sortKey="name" currentSort={sort} currentDirection={dir} onSort={handleSort} />
              <th className="px-4 py-2 text-xs text-text-secondary">Status</th>
              <SortableHeader label="Tasks" sortKey="step_count" currentSort={sort} currentDirection={dir} onSort={handleSort} />
              <th className="px-4 py-2 text-xs text-text-secondary hidden md:table-cell">Machine</th>
              <th className="px-4 py-2 text-xs text-text-secondary hidden md:table-cell">Trigger</th>
              <SortableHeader label="Created" sortKey="created_at" currentSort={sort} currentDirection={dir} onSort={handleSort} />
              <th className="px-4 py-2"></th>
            </tr>
          </thead>
          <tbody>
            {pagedJobs.map((job: Job) => (
              <tr
                key={job.job_id}
                onClick={() => navigate(`/jobs/${job.job_id}`)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    navigate(`/jobs/${job.job_id}`);
                  }
                }}
                tabIndex={0}
                role="row"
                aria-label={`Open job ${job.name}`}
                className="bg-bg-secondary hover:bg-bg-tertiary/50 cursor-pointer border-b border-border-primary/50 transition-colors focus:outline-none focus:ring-1 focus:ring-accent-primary"
              >
                <td className="px-4 py-2">
                  <div className="text-text-primary font-medium truncate">{job.name}</div>
                  {job.description && (
                    <div className="text-xs text-text-secondary truncate mt-0.5">{job.description}</div>
                  )}
                  <CopyableId id={job.job_id} className="text-xs" />
                </td>
                <td className="px-4 py-2">
                  {job.last_run_status ? (
                    <RunStatusBadge status={job.last_run_status} size="sm" />
                  ) : (
                    <span className="text-xs text-text-secondary/40">—</span>
                  )}
                </td>
                <td className="px-4 py-2 text-text-secondary">
                  {job.step_count ?? 0}
                </td>
                <td className="px-4 py-2 font-mono text-xs text-text-secondary hidden md:table-cell" title={job.machine_ids ?? ''}>
                  {formatMachineIds(job.machine_ids)}
                </td>
                <td className="px-4 py-2 hidden md:table-cell">
                  <TriggerBadge type={job.trigger_type ?? 'manual'} />
                </td>
                <td className="px-4 py-2 text-text-secondary hidden md:table-cell">
                  {formatTimeAgo(job.created_at)}
                </td>
                <td className="px-4 py-2">
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      type="button"
                      onClick={(e) => handleRun(e, job.job_id)}
                      className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-green-600/20 text-green-400 hover:bg-green-600/30 transition-colors"
                      title="Run job"
                    >
                      <Play size={14} />
                      Run
                    </button>
                    <button
                      type="button"
                      aria-label={`Duplicate job ${job.name}`}
                      onClick={(e) => handleClone(e, job)}
                      className="p-1.5 rounded-md text-text-secondary hover:text-accent-primary hover:bg-accent-primary/10 transition-colors"
                      title="Duplicate"
                    >
                      <CopyPlus size={14} />
                    </button>
                    <button
                      type="button"
                      aria-label={`Delete job ${job.name}`}
                      onClick={(e) => { e.stopPropagation(); setDeleteTarget(job); }}
                      className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
                      title="Delete"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <Pagination
          page={page}
          pageSize={pageSize}
          total={total}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
        />
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Job"
        message={`Are you sure you want to delete "${deleteTarget?.name ?? ''}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
