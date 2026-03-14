import type { Run } from '../../types/job.ts';
import { RunStatusBadge } from './RunStatusBadge.tsx';
import { DurationDisplay } from './DurationDisplay.tsx';
import { EmptyState } from '../shared/EmptyState.tsx';
import { formatTimeAgo, truncateId } from '../../lib/format.ts';

interface RunsTableProps {
  runs: Run[];
  showJobName?: boolean;
  compact?: boolean;
  onRowClick: (runId: string) => void;
}

function formatMachineIds(ids: string | undefined): string {
  if (!ids) return '—';
  const machines = ids.split(',');
  if (machines.length === 1) return machines[0].slice(0, 12);
  return `${machines[0].slice(0, 12)} +${machines.length - 1}`;
}

export function RunsTable({ runs, showJobName = false, compact = false, onRowClick }: RunsTableProps) {
  if (runs.length === 0) {
    return <EmptyState title="No runs found" description="No runs match the current filters." />;
  }

  const badgeSize = compact ? 'sm' as const : 'md' as const;

  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="text-left text-xs text-text-secondary border-b border-border-primary">
          {!compact && <th className="px-4 py-2">Run ID</th>}
          <th className="px-4 py-2">Status</th>
          {showJobName && !compact && <th className="px-4 py-2">Job Name</th>}
          {!compact && <th className="px-4 py-2">Machine</th>}
          <th className="px-4 py-2">Trigger</th>
          <th className="px-4 py-2">Started</th>
          <th className="px-4 py-2">Duration</th>
        </tr>
      </thead>
      <tbody>
        {runs.map((run) => (
          <tr
            key={run.run_id}
            onClick={() => onRowClick(run.run_id)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onRowClick(run.run_id);
              }
            }}
            tabIndex={0}
            role="button"
            className="bg-bg-secondary hover:bg-bg-tertiary/50 cursor-pointer border-b border-border-primary/50 transition-colors focus:outline-none focus:ring-1 focus:ring-accent-primary"
          >
            {!compact && (
              <td className="px-4 py-2 font-mono text-xs text-text-secondary/60" title={run.run_id}>
                {truncateId(run.run_id)}
              </td>
            )}
            <td className="px-4 py-2">
              <RunStatusBadge status={run.status} size={badgeSize} />
            </td>
            {showJobName && !compact && (
              <td className="px-4 py-2 text-text-primary">{run.job_name ?? run.job_id.slice(0, 8)}</td>
            )}
            {!compact && (
              <td className="px-4 py-2 font-mono text-xs text-text-secondary" title={run.machine_ids ?? ''}>
                {formatMachineIds(run.machine_ids)}
              </td>
            )}
            <td className="px-4 py-2 text-text-secondary">{run.trigger_type ?? 'manual'}</td>
            <td className="px-4 py-2 text-text-secondary">
              {run.started_at ? formatTimeAgo(run.started_at) : formatTimeAgo(run.created_at)}
            </td>
            <td className="px-4 py-2 text-text-secondary">
              <DurationDisplay startedAt={run.started_at} completedAt={run.completed_at} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
