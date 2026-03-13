interface SkeletonTableProps {
  rows?: number;
  columns?: number;
}

const CELL_WIDTHS = ['w-3/4', 'w-1/2', 'w-2/3', 'w-1/3', 'w-3/5'];

export function SkeletonTable({ rows = 5, columns = 4 }: SkeletonTableProps) {
  return (
    <div className="overflow-hidden rounded-lg border border-border-primary">
      <table className="w-full border-collapse">
        <thead>
          <tr className="bg-bg-secondary">
            {Array.from({ length: columns }, (_, col) => (
              <th key={col} className="px-4 py-3 text-left">
                <div className="h-3 bg-bg-tertiary animate-pulse rounded w-1/2" />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }, (_, row) => (
            <tr key={row} className="border-t border-border-primary">
              {Array.from({ length: columns }, (_, col) => (
                <td key={col} className="px-4 py-3">
                  <div
                    className={`h-3 bg-bg-tertiary animate-pulse rounded ${CELL_WIDTHS[(row + col) % CELL_WIDTHS.length]}`}
                  />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
