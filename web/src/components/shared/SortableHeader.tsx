import { ChevronUp, ChevronDown, ChevronsUpDown } from 'lucide-react';

interface SortableHeaderProps {
  label: string;
  sortKey: string;
  currentSort: string | null;
  currentDirection: 'asc' | 'desc';
  onSort: (key: string) => void;
}

export function SortableHeader({ label, sortKey, currentSort, currentDirection, onSort }: SortableHeaderProps) {
  const isActive = currentSort === sortKey;
  const ariaSort = isActive ? (currentDirection === 'asc' ? 'ascending' : 'descending') : undefined;

  return (
    <th className="px-4 py-3" aria-sort={ariaSort}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className="inline-flex items-center gap-1 text-xs font-semibold uppercase tracking-wider text-text-secondary hover:text-text-primary transition-colors cursor-pointer select-none"
      >
        {label}
        {isActive ? (currentDirection === 'asc' ? <ChevronUp size={14} /> : <ChevronDown size={14} />) : <ChevronsUpDown size={12} className="opacity-30" />}
      </button>
    </th>
  );
}
