import { useState, useMemo, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { AlertCircle, Play, RefreshCw } from 'lucide-react';
import { useRuns } from '../hooks/useRuns.ts';
import { useJobs } from '../hooks/useJobs.ts';
import { RunsTable } from '../components/runs/RunsTable.tsx';
import { RunFilters } from '../components/runs/RunFilters.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { RefreshButton } from '../components/shared/RefreshButton.tsx';
import { Pagination } from '../components/shared/Pagination.tsx';
import { usePagination } from '../hooks/usePagination.ts';
import type { ListRunsParams } from '../types/job.ts';

export function RunsPage() {
  const navigate = useNavigate();

  const [selectedJobId, setSelectedJobId] = useState('all');
  const [selectedStatus, setSelectedStatus] = useState('all');
  const [selectedTriggerType, setSelectedTriggerType] = useState('all');

  const params = useMemo<ListRunsParams>(() => {
    const p: ListRunsParams = {};
    if (selectedJobId !== 'all') p.job_id = selectedJobId;
    if (selectedStatus !== 'all') p.status = selectedStatus;
    if (selectedTriggerType !== 'all') p.trigger_type = selectedTriggerType;
    return p;
  }, [selectedJobId, selectedStatus, selectedTriggerType]);

  const { data: runs, isLoading, isFetching, error, refetch } = useRuns(params);
  const { data: jobs } = useJobs();

  const { paged: pagedRuns, page, pageSize, total, setPage, setPageSize } = usePagination(runs ?? []);

  useEffect(() => { setPage(1); }, [selectedJobId, selectedStatus, selectedTriggerType, setPage]);

  return (
    <div className="p-4 md:p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Runs</h1>
        <RefreshButton onClick={() => refetch()} loading={isFetching} />
      </div>

      {/* Filters */}
      <RunFilters
        jobs={jobs ?? []}
        selectedJobId={selectedJobId}
        selectedStatus={selectedStatus}
        selectedTriggerType={selectedTriggerType}
        onJobChange={setSelectedJobId}
        onStatusChange={setSelectedStatus}
        onTriggerTypeChange={setSelectedTriggerType}
      />

      {/* Error State */}
      {error && (
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load runs'}
          </p>
          <button
            onClick={() => refetch()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      )}

      {/* Loading State */}
      {isLoading && <SkeletonTable rows={5} columns={5} />}

      {/* Runs Table */}
      {!isLoading && !error && (
        <>
          {(runs ?? []).length === 0 ? (
            <EmptyState
              icon={<Play size={40} />}
              title="No runs yet"
              description="Runs will appear here when jobs are triggered manually or by automation."
            />
          ) : (
            <>
              <RunsTable
                runs={pagedRuns}
                showJobName={selectedJobId === 'all'}
                onRowClick={(runId) => navigate(`/runs/${runId}`)}
              />
              <Pagination
                page={page}
                pageSize={pageSize}
                total={total}
                onPageChange={setPage}
                onPageSizeChange={setPageSize}
              />
            </>
          )}
        </>
      )}
    </div>
  );
}
