interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  pageSizeOptions?: number[];
}

export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [25, 50, 100],
}: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const start = Math.min((page - 1) * pageSize + 1, total);
  const end = Math.min(page * pageSize, total);

  if (total === 0) return null;

  return (
    <div className="flex items-center justify-between px-4 py-3 text-sm text-text-secondary border-t border-border-primary">
      <span>
        Showing {start}&ndash;{end} of {total}
      </span>
      <div className="flex items-center gap-3">
        <select
          value={pageSize}
          onChange={(e) => {
            onPageSizeChange(Number(e.target.value));
            onPageChange(1);
          }}
          aria-label="Rows per page"
          className="bg-bg-tertiary border border-border-primary rounded px-2 py-1 text-xs text-text-primary"
        >
          {pageSizeOptions.map((s) => (
            <option key={s} value={s}>
              {s} per page
            </option>
          ))}
        </select>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => onPageChange(page - 1)}
            disabled={page <= 1}
            className="px-2.5 py-1 rounded text-xs bg-bg-tertiary hover:bg-bg-tertiary/80 disabled:opacity-30 transition-colors"
          >
            Prev
          </button>
          <span className="px-2 text-xs">
            {page} / {totalPages}
          </span>
          <button
            type="button"
            onClick={() => onPageChange(page + 1)}
            disabled={page >= totalPages}
            className="px-2.5 py-1 rounded text-xs bg-bg-tertiary hover:bg-bg-tertiary/80 disabled:opacity-30 transition-colors"
          >
            Next
          </button>
        </div>
      </div>
    </div>
  );
}
