import type { ReactNode } from 'react';
import { useIsMobile } from '../../hooks/useMediaQuery.ts';

interface Field<T> {
  label: string;
  render: (item: T) => ReactNode;
  /** If true, this field is hidden in card view (e.g. redundant with card header). */
  hideInCard?: boolean;
}

interface ResponsiveTableProps<T> {
  items: T[];
  keyFn: (item: T) => string;
  fields: Field<T>[];
  /** Optional: custom table header row (overrides auto-generated headers). */
  tableHead?: ReactNode;
  /** Optional: custom table row renderer (overrides auto-generated rows). */
  tableRow?: (item: T) => ReactNode;
  /** Actions column rendered at the end of each row / bottom of each card. */
  actions?: (item: T) => ReactNode;
  /** Click handler for the whole row/card. */
  onItemClick?: (item: T) => void;
}

/**
 * Renders a table on md+ screens and a stacked card list on mobile.
 * Desktop rendering is identical to a plain table — zero visual change.
 */
export function ResponsiveTable<T>({
  items,
  keyFn,
  fields,
  tableHead,
  tableRow,
  actions,
  onItemClick,
}: ResponsiveTableProps<T>) {
  const isMobile = useIsMobile();

  if (!isMobile) {
    // Desktop: classic table
    return (
      <div className="overflow-x-auto rounded-lg border border-border-primary">
        <table className="w-full border-collapse text-left">
          {tableHead ?? (
            <thead>
              <tr className="bg-bg-secondary">
                {fields.map((f) => (
                  <th
                    key={f.label}
                    className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary"
                  >
                    {f.label}
                  </th>
                ))}
                {actions && <th className="px-4 py-3" />}
              </tr>
            </thead>
          )}
          <tbody>
            {items.map((item) =>
              tableRow ? (
                tableRow(item)
              ) : (
                <tr
                  key={keyFn(item)}
                  onClick={onItemClick ? () => onItemClick(item) : undefined}
                  className={`border-t border-gray-800 hover:bg-bg-tertiary/50 transition-colors ${onItemClick ? 'cursor-pointer' : ''}`}
                >
                  {fields.map((f) => (
                    <td key={f.label} className="px-4 py-3 text-sm">
                      {f.render(item)}
                    </td>
                  ))}
                  {actions && (
                    <td className="px-4 py-3">
                      {actions(item)}
                    </td>
                  )}
                </tr>
              ),
            )}
          </tbody>
        </table>
      </div>
    );
  }

  // Mobile: stacked cards
  return (
    <div className="space-y-3">
      {items.map((item) => (
        <div
          key={keyFn(item)}
          onClick={onItemClick ? () => onItemClick(item) : undefined}
          className={`rounded-lg border border-border-primary bg-bg-secondary p-4 space-y-2 ${onItemClick ? 'cursor-pointer active:bg-bg-tertiary/30' : ''}`}
        >
          {fields
            .filter((f) => !f.hideInCard)
            .map((f) => (
              <div key={f.label} className="flex items-start justify-between gap-2">
                <span className="text-xs text-text-secondary uppercase tracking-wider shrink-0">
                  {f.label}
                </span>
                <span className="text-sm text-text-primary text-right">{f.render(item)}</span>
              </div>
            ))}
          {actions && (
            <div className="flex items-center justify-end gap-2 pt-2 border-t border-border-primary">
              {actions(item)}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
