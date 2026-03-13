import { Search } from 'lucide-react';

export const LIMIT_OPTIONS = [25, 50, 100] as const;
export type LimitOption = (typeof LIMIT_OPTIONS)[number];

export interface EventFilterValues {
  typePattern: string;
  since: string;
  limit: LimitOption;
}

interface EventFiltersProps {
  filters: EventFilterValues;
  onChange: (filters: EventFilterValues) => void;
}

export function EventFilters({ filters, onChange }: EventFiltersProps) {
  function handleTypeChange(e: React.ChangeEvent<HTMLInputElement>) {
    onChange({ ...filters, typePattern: e.target.value });
  }

  function handleSinceChange(e: React.ChangeEvent<HTMLInputElement>) {
    onChange({ ...filters, since: e.target.value });
  }

  function handleLimitChange(e: React.ChangeEvent<HTMLSelectElement>) {
    onChange({ ...filters, limit: Number(e.target.value) as LimitOption });
  }

  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="relative flex-1 min-w-48">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary pointer-events-none" />
        <input
          type="text"
          placeholder="Filter by event type (e.g. run.*)"
          value={filters.typePattern}
          onChange={handleTypeChange}
          className="w-full pl-8 pr-3 py-2 text-sm bg-bg-secondary border border-gray-700 rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary transition-colors"
        />
      </div>

      <div className="flex items-center gap-2">
        <label className="text-xs text-text-secondary whitespace-nowrap">Since</label>
        <input
          type="datetime-local"
          value={filters.since}
          onChange={handleSinceChange}
          className="px-3 py-2 text-sm bg-bg-secondary border border-gray-700 rounded-md text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
        />
      </div>

      <div className="flex items-center gap-2">
        <label className="text-xs text-text-secondary whitespace-nowrap">Per page</label>
        <select
          value={filters.limit}
          onChange={handleLimitChange}
          className="px-3 py-2 text-sm bg-bg-secondary border border-gray-700 rounded-md text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
        >
          {LIMIT_OPTIONS.map((opt) => (
            <option key={opt} value={opt}>
              {opt}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}
