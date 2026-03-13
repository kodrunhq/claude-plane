import { useState } from 'react';
import { Activity, AlertCircle, ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import { useEvents } from '../hooks/useEvents.ts';
import { EventsTable } from '../components/events/EventsTable.tsx';
import { EventFilters, LIMIT_OPTIONS } from '../components/events/EventFilters.tsx';
import type { EventFilterValues } from '../components/events/EventFilters.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';

const DEFAULT_FILTERS: EventFilterValues = {
  typePattern: '',
  since: '',
  limit: LIMIT_OPTIONS[0],
};

function localDateTimeToRFC3339(localDT: string): string {
  if (!localDT) return '';
  return new Date(localDT).toISOString();
}

export function EventsPage() {
  const [filters, setFilters] = useState<EventFilterValues>(DEFAULT_FILTERS);
  const [offset, setOffset] = useState(0);

  const queryParams = {
    type: filters.typePattern || undefined,
    since: filters.since ? localDateTimeToRFC3339(filters.since) : undefined,
    limit: filters.limit,
    offset,
  };

  const { data: events, isLoading, error, refetch, isFetching } = useEvents(queryParams);

  const totalLoaded = events?.length ?? 0;
  const hasNextPage = totalLoaded === filters.limit;
  const hasPrevPage = offset > 0;
  const currentPage = Math.floor(offset / filters.limit) + 1;

  function handleFiltersChange(next: EventFilterValues) {
    setFilters(next);
    setOffset(0);
  }

  function handlePrev() {
    setOffset(Math.max(0, offset - filters.limit));
  }

  function handleNext() {
    setOffset(offset + filters.limit);
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load events'}
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
        <div>
          <h1 className="text-xl font-semibold text-text-primary">Event Log</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Audit history of all system events
          </p>
        </div>
        <button
          onClick={() => refetch()}
          disabled={isFetching}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors disabled:opacity-50"
          title="Refresh"
        >
          <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      <EventFilters filters={filters} onChange={handleFiltersChange} />

      {isLoading ? (
        <SkeletonTable rows={10} columns={5} />
      ) : !events || events.length === 0 ? (
        <EmptyState
          icon={<Activity size={40} />}
          title="No events found"
          description={
            filters.typePattern || filters.since
              ? 'Try adjusting your filters to see more events.'
              : 'No events have been recorded yet.'
          }
        />
      ) : (
        <>
          <EventsTable events={events} />

          <div className="flex items-center justify-between pt-2">
            <span className="text-xs text-text-secondary">
              Page {currentPage} &mdash; showing {totalLoaded} event{totalLoaded !== 1 ? 's' : ''}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={handlePrev}
                disabled={!hasPrevPage}
                className="flex items-center gap-1 px-3 py-1.5 text-xs rounded-md bg-bg-secondary border border-gray-700 text-text-secondary hover:text-text-primary hover:border-gray-500 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              >
                <ChevronLeft size={14} />
                Prev
              </button>
              <button
                onClick={handleNext}
                disabled={!hasNextPage}
                className="flex items-center gap-1 px-3 py-1.5 text-xs rounded-md bg-bg-secondary border border-gray-700 text-text-secondary hover:text-text-primary hover:border-gray-500 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Next
                <ChevronRight size={14} />
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
