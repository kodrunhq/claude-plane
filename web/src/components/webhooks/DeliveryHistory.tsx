import { useState } from 'react';
import { ChevronDown, ChevronRight, RefreshCw } from 'lucide-react';
import { useWebhookDeliveries } from '../../hooks/useWebhooks.ts';
import { SkeletonTable } from '../shared/SkeletonTable.tsx';
import { EmptyState } from '../shared/EmptyState.tsx';
import { formatTimeAgo } from '../../lib/format.ts';
import type { WebhookDelivery } from '../../types/webhook.ts';

interface DeliveryHistoryProps {
  webhookId: string;
}

const STATUS_COLORS: Record<string, string> = {
  delivered: 'text-status-success',
  failed: 'text-status-error',
  pending: 'text-status-running',
  retrying: 'text-yellow-400',
};

function DeliveryRow({ delivery }: { delivery: WebhookDelivery }) {
  const [expanded, setExpanded] = useState(false);

  const statusColor = STATUS_COLORS[delivery.status] ?? 'text-text-secondary';
  const hasDetails = !!(delivery.last_error || delivery.next_retry_at || delivery.payload);

  return (
    <>
      <tr
        className={`border-t border-gray-800 ${hasDetails ? 'cursor-pointer hover:bg-bg-tertiary/50' : ''}`}
        onClick={() => hasDetails && setExpanded((v) => !v)}
      >
        <td className="px-4 py-3">
          <span className={`text-sm font-medium ${statusColor} capitalize`}>
            {delivery.status}
          </span>
        </td>
        <td className="px-4 py-3 text-sm text-text-secondary font-mono">
          {delivery.response_code ?? '—'}
        </td>
        <td className="px-4 py-3 text-sm text-text-secondary">{delivery.attempts}</td>
        <td className="px-4 py-3 text-sm text-text-secondary">
          {formatTimeAgo(delivery.created_at)}
        </td>
        <td className="px-4 py-3 text-sm text-text-secondary font-mono truncate max-w-[200px]">
          {delivery.event_id}
        </td>
        <td className="px-4 py-3 text-right">
          {hasDetails && (
            <span className="text-text-secondary">
              {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            </span>
          )}
        </td>
      </tr>
      {expanded && hasDetails && (
        <tr className="bg-bg-tertiary/20">
          <td colSpan={6} className="px-4 py-3">
            <div className="space-y-2 text-xs">
              {delivery.last_error && (
                <div>
                  <span className="text-text-secondary font-medium">Error: </span>
                  <span className="text-status-error font-mono">{delivery.last_error}</span>
                </div>
              )}
              {delivery.next_retry_at && (
                <div>
                  <span className="text-text-secondary font-medium">Next retry: </span>
                  <span className="text-text-primary">{formatTimeAgo(delivery.next_retry_at)}</span>
                </div>
              )}
              {delivery.payload && (
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-text-secondary font-medium">Payload:</span>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        navigator.clipboard?.writeText(delivery.payload ?? '')?.catch(() => {
                          // Clipboard API requires HTTPS; silently ignore on HTTP deployments.
                        });
                      }}
                      className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
                    >
                      Copy
                    </button>
                  </div>
                  <pre className="bg-bg-primary rounded p-2 text-xs font-mono text-text-primary overflow-x-auto max-h-48 whitespace-pre-wrap">
                    {(() => {
                      try {
                        return JSON.stringify(JSON.parse(delivery.payload), null, 2);
                      } catch {
                        return delivery.payload;
                      }
                    })()}
                  </pre>
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

export function DeliveryHistory({ webhookId }: DeliveryHistoryProps) {
  const { data: deliveries, isLoading, error, refetch } = useWebhookDeliveries(webhookId);

  if (isLoading) {
    return <SkeletonTable rows={5} columns={6} />;
  }

  if (error) {
    return (
      <div className="flex items-center justify-between p-4 bg-status-error/10 border border-status-error/30 rounded-lg">
        <p className="text-sm text-text-primary">
          {error instanceof Error ? error.message : 'Failed to load delivery history'}
        </p>
        <button
          onClick={() => refetch()}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
        >
          <RefreshCw size={12} />
          Retry
        </button>
      </div>
    );
  }

  if (!deliveries || deliveries.length === 0) {
    return (
      <EmptyState
        title="No deliveries yet"
        description="Deliveries will appear here after the webhook receives events."
      />
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border-primary">
      <table className="w-full border-collapse text-left">
        <thead>
          <tr className="bg-bg-secondary">
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
              Status
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
              Response
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
              Attempts
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
              Created
            </th>
            <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
              Event ID
            </th>
            <th className="px-4 py-3" />
          </tr>
        </thead>
        <tbody>
          {deliveries.map((delivery) => (
            <DeliveryRow key={delivery.delivery_id} delivery={delivery} />
          ))}
        </tbody>
      </table>
    </div>
  );
}
