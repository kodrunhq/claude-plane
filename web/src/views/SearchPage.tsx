import { useState, useEffect, useMemo } from 'react';
import { Link } from 'react-router';
import { Search, AlertCircle, Loader2, FileSearch } from 'lucide-react';
import { useSessionSearch } from '../hooks/useSearch.ts';
import type { SearchResult } from '../api/search.ts';

function formatRelativeTime(timestampMs: number): string {
  const now = Date.now();
  const diffMs = now - timestampMs;
  const seconds = Math.floor(diffMs / 1000);

  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function HighlightedLine({ text, query }: { text: string; query: string }) {
  if (!query) return <span>{text}</span>;

  const parts = text.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi'));

  return (
    <span>
      {parts.map((part, i) =>
        part.toLowerCase() === query.toLowerCase() ? (
          <mark key={i} className="bg-accent-primary/30 text-text-primary rounded px-0.5">
            {part}
          </mark>
        ) : (
          <span key={i}>{part}</span>
        ),
      )}
    </span>
  );
}

function ResultItem({ result, query }: { result: SearchResult; query: string }) {
  return (
    <div className="px-4 py-3 hover:bg-bg-tertiary/40 transition-colors">
      <div className="flex items-center justify-between mb-1.5">
        <div className="flex items-center gap-2 min-w-0">
          <Link
            to={`/sessions/${result.session_id}`}
            className="text-sm font-medium text-accent-primary hover:text-accent-primary/80 transition-colors truncate"
          >
            {result.session_id}
          </Link>
          {result.session_status && (
            <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-bg-tertiary text-text-secondary uppercase tracking-wide">
              {result.session_status}
            </span>
          )}
        </div>
        <div className="flex items-center gap-3 shrink-0 ml-3">
          <span className="text-xs text-text-secondary">{result.machine_id}</span>
          <span className="text-xs text-text-secondary tabular-nums">
            {formatRelativeTime(result.timestamp_ms)}
          </span>
        </div>
      </div>

      {result.context_before && (
        <p className="text-xs text-text-secondary/60 font-mono truncate pl-2 border-l-2 border-border-primary">
          {result.context_before}
        </p>
      )}
      <p className="text-sm text-text-primary font-mono truncate pl-2 border-l-2 border-accent-primary/50 my-0.5">
        <HighlightedLine text={result.line} query={query} />
      </p>
      {result.context_after && (
        <p className="text-xs text-text-secondary/60 font-mono truncate pl-2 border-l-2 border-border-primary">
          {result.context_after}
        </p>
      )}
    </div>
  );
}

export function SearchPage() {
  const [inputValue, setInputValue] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(inputValue.trim());
    }, 300);
    return () => clearTimeout(timer);
  }, [inputValue]);

  const { data: results, isLoading, error, isFetching } = useSessionSearch(debouncedQuery);

  const hasQuery = debouncedQuery.length >= 2;

  const sortedResults = useMemo(
    () => [...(results ?? [])].sort((a, b) => b.timestamp_ms - a.timestamp_ms),
    [results],
  );

  return (
    <div className="p-4 md:p-6 space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-xl font-semibold text-text-primary">Search Session Logs</h1>
        <p className="text-sm text-text-secondary mt-1">
          Search across session output from all connected machines
        </p>
      </div>

      {/* Search Input */}
      <div className="relative">
        <Search
          size={18}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary pointer-events-none"
        />
        <input
          type="text"
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          placeholder="Search session logs..."
          className="w-full pl-10 pr-4 py-2.5 rounded-lg bg-bg-secondary border border-border-primary text-sm text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-2 focus:ring-accent-primary/40 focus:border-accent-primary/60 transition-all"
          autoFocus
        />
        {isFetching && (
          <Loader2
            size={16}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-text-secondary animate-spin"
          />
        )}
      </div>

      {/* Error State */}
      {error && (
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary">
            {error instanceof Error ? error.message : 'Search failed. Please try again.'}
          </p>
        </div>
      )}

      {/* Initial State */}
      {!hasQuery && !error && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <FileSearch size={48} className="text-text-secondary/30 mb-4" />
          <p className="text-sm text-text-secondary">
            Search session logs across all connected machines
          </p>
          <p className="text-xs text-text-secondary/60 mt-1">
            Enter at least 2 characters to start searching
          </p>
        </div>
      )}

      {/* Loading State */}
      {hasQuery && isLoading && (
        <div className="bg-bg-secondary rounded-lg border border-border-primary divide-y divide-border-primary">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="px-4 py-3 animate-pulse">
              <div className="flex items-center gap-2 mb-2">
                <div className="h-4 bg-bg-tertiary rounded w-40" />
                <div className="h-3 bg-bg-tertiary rounded w-20 ml-auto" />
              </div>
              <div className="h-3 bg-bg-tertiary rounded w-3/4" />
            </div>
          ))}
        </div>
      )}

      {/* Results */}
      {hasQuery && !isLoading && !error && sortedResults.length > 0 && (
        <div>
          <p className="text-xs text-text-secondary mb-2">
            {sortedResults.length} result{sortedResults.length !== 1 ? 's' : ''} found
          </p>
          <div className="bg-bg-secondary rounded-lg border border-border-primary divide-y divide-border-primary">
            {sortedResults.map((result, i) => (
              <ResultItem key={`${result.session_id}-${result.timestamp_ms}-${i}`} result={result} query={debouncedQuery} />
            ))}
          </div>
        </div>
      )}

      {/* Empty State */}
      {hasQuery && !isLoading && !error && sortedResults.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <Search size={48} className="text-text-secondary/30 mb-4" />
          <p className="text-sm text-text-secondary">No results found</p>
          <p className="text-xs text-text-secondary/60 mt-1">
            Try a different search term or check that sessions have output
          </p>
        </div>
      )}
    </div>
  );
}
