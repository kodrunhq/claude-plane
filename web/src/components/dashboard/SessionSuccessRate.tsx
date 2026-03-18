import { BarChart3 } from 'lucide-react';
import { useSessionStats } from '../../hooks/useLogs.ts';

export function SessionSuccessRate() {
  const { data: stats, isLoading } = useSessionStats();

  const total = stats?.total ?? 0;
  const succeeded = stats?.succeeded ?? 0;
  const rate = total > 0 ? Math.round((succeeded / total) * 100) : 0;

  return (
    <div className="bg-bg-secondary border border-border-primary rounded-lg p-4 space-y-3">
      <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider flex items-center gap-2">
        <BarChart3 size={16} />
        Session Success Rate
      </h3>

      {isLoading ? (
        <div className="animate-pulse space-y-2">
          <div className="h-4 bg-bg-tertiary rounded w-1/2" />
          <div className="h-3 bg-bg-tertiary rounded-full w-full" />
        </div>
      ) : total === 0 ? (
        <p className="text-sm text-text-secondary">No session data available.</p>
      ) : (
        <>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold text-text-primary font-mono tabular-nums">
              {rate}%
            </span>
            <span className="text-xs text-text-secondary">
              {succeeded}/{total} sessions succeeded ({stats?.period ?? 'last 24h'})
            </span>
          </div>
          <div className="h-2.5 bg-bg-tertiary rounded-full overflow-hidden">
            <div
              className="h-full bg-green-500 rounded-full transition-all duration-500"
              style={{ width: `${rate}%` }}
            />
          </div>
        </>
      )}
    </div>
  );
}
