import { AlertCircle, ScrollText, RefreshCw, Wifi, WifiOff } from 'lucide-react';
import { useLogsStore } from '../stores/logs.ts';
import { useLogs } from '../hooks/useLogs.ts';
import { useLogStream } from '../hooks/useLogStream.ts';
import { LogFilters } from '../components/logs/LogFilters.tsx';
import { LogTable } from '../components/logs/LogTable.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { Pagination } from '../components/shared/Pagination.tsx';
import { RefreshButton } from '../components/shared/RefreshButton.tsx';
import { usePagination } from '../hooks/usePagination.ts';

export function LogsPage() {
  const filter = useLogsStore((s) => s.filter);
  const live = useLogsStore((s) => s.live);

  // REST query for non-live mode
  const { data, isLoading, error, refetch, isFetching } = useLogs(filter);

  // WebSocket stream for live mode
  const { entries: liveEntries, connected } = useLogStream(filter, live);

  // Determine which entries to show
  const restEntries = data?.logs ?? [];
  const displayEntries = live ? liveEntries : restEntries;
  const totalCount = live ? liveEntries.length : (data?.total ?? 0);

  // Pagination for non-live mode
  const {
    paged: pagedEntries,
    page,
    pageSize,
    total: paginatedTotal,
    setPage,
    setPageSize,
  } = usePagination(displayEntries, 50);

  const hasActiveFilters = Boolean(
    filter.level || filter.source || filter.component || filter.machine_id || filter.search || filter.since,
  );

  if (error && !live) {
    return (
      <div className="p-4 md:p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load logs'}
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
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold text-text-primary">System Logs</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Structured logs from server and agent components
          </p>
        </div>
        <div className="flex items-center gap-2">
          {live && (
            <span className="flex items-center gap-1.5 text-xs text-text-secondary">
              {connected ? (
                <>
                  <Wifi size={14} className="text-green-400" />
                  <span className="text-green-400">Connected</span>
                </>
              ) : (
                <>
                  <WifiOff size={14} className="text-amber-400" />
                  <span className="text-amber-400">Reconnecting...</span>
                </>
              )}
            </span>
          )}
          {!live && (
            <span className="text-xs text-text-secondary">
              {totalCount} log{totalCount !== 1 ? 's' : ''}
            </span>
          )}
          {!live && <RefreshButton onClick={() => refetch()} loading={isFetching} />}
        </div>
      </div>

      {/* Filters */}
      <LogFilters />

      {/* Content */}
      {isLoading && !live ? (
        <SkeletonTable rows={10} columns={6} />
      ) : displayEntries.length === 0 ? (
        <EmptyState
          icon={<ScrollText size={40} />}
          title="No logs found"
          description={
            hasActiveFilters
              ? 'Try adjusting your filters to see more logs.'
              : live
                ? 'Waiting for new log entries...'
                : 'No log entries have been recorded yet.'
          }
        />
      ) : (
        <>
          <LogTable entries={live ? displayEntries : pagedEntries} loading={false} />

          {!live && (
            <Pagination
              page={page}
              pageSize={pageSize}
              total={paginatedTotal}
              onPageChange={setPage}
              onPageSizeChange={setPageSize}
              pageSizeOptions={[25, 50, 100]}
            />
          )}
        </>
      )}
    </div>
  );
}
