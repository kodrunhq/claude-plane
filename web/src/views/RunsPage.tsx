import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { useRuns } from '../hooks/useRuns.ts';
import { useJobs } from '../hooks/useJobs.ts';
import { RunsTable } from '../components/runs/RunsTable.tsx';
import { RunFilters } from '../components/runs/RunFilters.tsx';
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

  const { data: runs, isLoading, error, refetch } = useRuns(params);
  const { data: jobs } = useJobs();

  return (
    <div className="p-4 md:p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Runs</h1>
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
      {isLoading && (
        <div className="space-y-2">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-bg-tertiary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-secondary rounded w-1/4 mb-2" />
              <div className="h-3 bg-bg-secondary rounded w-1/2" />
            </div>
          ))}
        </div>
      )}

      {/* Runs Table */}
      {!isLoading && !error && (
        <>
          {(runs ?? []).length === 0 ? (
            <p className="text-sm text-text-secondary">No runs yet</p>
          ) : (
            <RunsTable
              runs={runs ?? []}
              showJobName={selectedJobId === 'all'}
              onRowClick={(runId) => navigate(`/runs/${runId}`)}
            />
          )}
        </>
      )}
    </div>
  );
}
