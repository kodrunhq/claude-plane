import { useState } from 'react';
import { Pencil, Trash2, History } from 'lucide-react';
import { ConfirmDialog } from '../shared/ConfirmDialog.tsx';
import { formatTimeAgo, truncateId } from '../../lib/format.ts';
import { useUpdateWebhook, useDeleteWebhook } from '../../hooks/useWebhooks.ts';
import { toast } from 'sonner';
import type { Webhook } from '../../types/webhook.ts';

interface WebhooksListProps {
  webhooks: Webhook[];
  onEdit: (webhook: Webhook) => void;
  onViewDeliveries: (webhook: Webhook) => void;
}

interface WebhookRowProps {
  webhook: Webhook;
  onEdit: (webhook: Webhook) => void;
  onViewDeliveries: (webhook: Webhook) => void;
  onDeleteRequest: (webhook: Webhook) => void;
}

function WebhookRow({ webhook, onEdit, onViewDeliveries, onDeleteRequest }: WebhookRowProps) {
  const updateWebhook = useUpdateWebhook();

  async function handleToggleEnabled(e: React.MouseEvent) {
    e.stopPropagation();
    try {
      await updateWebhook.mutateAsync({
        id: webhook.webhook_id,
        params: {
          name: webhook.name,
          url: webhook.url,
          events: webhook.events,
          enabled: !webhook.enabled,
        },
      });
      toast.success(webhook.enabled ? 'Webhook disabled' : 'Webhook enabled');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update webhook');
    }
  }

  return (
    <tr className="border-t border-gray-800 hover:bg-bg-tertiary/20 transition-colors">
      <td className="px-4 py-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium text-text-primary">{webhook.name}</span>
          <span className="text-xs text-text-secondary font-mono">{truncateId(webhook.webhook_id)}</span>
        </div>
      </td>
      <td className="px-4 py-3 text-sm text-text-secondary font-mono truncate max-w-[220px]">
        {webhook.url}
      </td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1">
          {webhook.events.slice(0, 3).map((event) => (
            <span
              key={event}
              className="px-1.5 py-0.5 text-xs bg-bg-tertiary text-text-secondary rounded font-mono"
            >
              {event}
            </span>
          ))}
          {webhook.events.length > 3 && (
            <span className="px-1.5 py-0.5 text-xs bg-bg-tertiary text-text-secondary rounded">
              +{webhook.events.length - 3}
            </span>
          )}
        </div>
      </td>
      <td className="px-4 py-3">
        <button
          onClick={handleToggleEnabled}
          disabled={updateWebhook.isPending}
          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none disabled:opacity-50 ${
            webhook.enabled ? 'bg-accent-primary' : 'bg-gray-600'
          }`}
          role="switch"
          aria-checked={webhook.enabled}
          title={webhook.enabled ? 'Disable webhook' : 'Enable webhook'}
        >
          <span
            className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
              webhook.enabled ? 'translate-x-[18px]' : 'translate-x-0.5'
            }`}
          />
        </button>
      </td>
      <td className="px-4 py-3 text-sm text-text-secondary">
        {formatTimeAgo(webhook.updated_at)}
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1 justify-end">
          <button
            onClick={() => onViewDeliveries(webhook)}
            className="p-1.5 rounded text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
            title="Delivery history"
          >
            <History size={15} />
          </button>
          <button
            onClick={() => onEdit(webhook)}
            className="p-1.5 rounded text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
            title="Edit webhook"
          >
            <Pencil size={15} />
          </button>
          <button
            onClick={() => onDeleteRequest(webhook)}
            className="p-1.5 rounded text-text-secondary hover:text-status-error hover:bg-bg-tertiary transition-colors"
            title="Delete webhook"
          >
            <Trash2 size={15} />
          </button>
        </div>
      </td>
    </tr>
  );
}

export function WebhooksList({ webhooks, onEdit, onViewDeliveries }: WebhooksListProps) {
  const [pendingDelete, setPendingDelete] = useState<Webhook | null>(null);
  const deleteWebhook = useDeleteWebhook();

  async function handleConfirmDelete() {
    if (!pendingDelete) return;
    try {
      await deleteWebhook.mutateAsync(pendingDelete.webhook_id);
      toast.success('Webhook deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete webhook');
    } finally {
      setPendingDelete(null);
    }
  }

  return (
    <>
      <div className="overflow-hidden rounded-lg border border-border-primary">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="bg-bg-secondary">
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Name
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                URL
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Events
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Enabled
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Updated
              </th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {webhooks.map((webhook) => (
              <WebhookRow
                key={webhook.webhook_id}
                webhook={webhook}
                onEdit={onEdit}
                onViewDeliveries={onViewDeliveries}
                onDeleteRequest={setPendingDelete}
              />
            ))}
          </tbody>
        </table>
      </div>

      <ConfirmDialog
        open={!!pendingDelete}
        title="Delete webhook"
        message={`Are you sure you want to delete "${pendingDelete?.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleConfirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </>
  );
}
