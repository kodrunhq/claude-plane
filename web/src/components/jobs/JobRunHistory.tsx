import { useRuns } from '../../hooks/useRuns.ts';
import { RunsTable } from '../runs/RunsTable.tsx';

interface JobRunHistoryProps {
  jobId: string;
  onRunClick: (runId: string) => void;
}

export function JobRunHistory({ jobId, onRunClick }: JobRunHistoryProps) {
  const { data: runs, isLoading } = useRuns({ job_id: jobId });

  if (isLoading) {
    return (
      <div className="px-4 py-3 text-sm text-text-secondary">Loading run history...</div>
    );
  }

  if (!runs || runs.length === 0) {
    return (
      <div className="px-4 py-3 text-sm text-text-secondary">No runs yet.</div>
    );
  }

  return (
    <RunsTable
      runs={runs}
      compact={true}
      showJobName={false}
      onRowClick={onRunClick}
    />
  );
}
