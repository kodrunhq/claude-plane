import { useState } from 'react';
import { useParams, useNavigate } from 'react-router';
import { Plus, Webhook, AlertCircle, RefreshCw, ArrowLeft } from 'lucide-react';
import { toast } from 'sonner';
import { useWebhooks, useCreateWebhook, useUpdateWebhook } from '../hooks/useWebhooks.ts';
import { WebhooksList } from '../components/webhooks/WebhooksList.tsx';
import { WebhookForm } from '../components/webhooks/WebhookForm.tsx';
import { DeliveryHistory } from '../components/webhooks/DeliveryHistory.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import type { Webhook as WebhookType, CreateWebhookParams, UpdateWebhookParams } from '../types/webhook.ts';

type DrawerMode = 'create' | 'edit' | 'deliveries' | null;

// Deliveries sub-page routed via /webhooks/:id/deliveries
export function WebhookDeliveriesPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: webhooks } = useWebhooks();
  const webhook = webhooks?.find((w) => w.webhook_id === id);

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center gap-3">
        <button
          onClick={() => navigate('/webhooks')}
          className="p-1.5 rounded text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
          title="Back to webhooks"
        >
          <ArrowLeft size={18} />
        </button>
        <div>
          <h1 className="text-xl font-semibold text-text-primary">Delivery History</h1>
          {webhook && (
            <p className="text-sm text-text-secondary mt-0.5">{webhook.name}</p>
          )}
        </div>
      </div>

      {id && <DeliveryHistory webhookId={id} />}
    </div>
  );
}

export function WebhooksPage() {
  const navigate = useNavigate();
  const { data: webhooks, isLoading, error, refetch } = useWebhooks();
  const createWebhook = useCreateWebhook();
  const updateWebhook = useUpdateWebhook();

  const [drawerMode, setDrawerMode] = useState<DrawerMode>(null);
  const [editingWebhook, setEditingWebhook] = useState<WebhookType | null>(null);

  function openCreate() {
    setEditingWebhook(null);
    setDrawerMode('create');
  }

  function openEdit(webhook: WebhookType) {
    setEditingWebhook(webhook);
    setDrawerMode('edit');
  }

  function openDeliveries(webhook: WebhookType) {
    navigate(`/webhooks/${webhook.webhook_id}/deliveries`);
  }

  function closeDrawer() {
    setDrawerMode(null);
    setEditingWebhook(null);
  }

  async function handleCreate(params: CreateWebhookParams | UpdateWebhookParams) {
    try {
      await createWebhook.mutateAsync(params as CreateWebhookParams);
      toast.success('Webhook created');
      closeDrawer();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create webhook');
    }
  }

  async function handleUpdate(params: CreateWebhookParams | UpdateWebhookParams) {
    if (!editingWebhook) return;
    try {
      await updateWebhook.mutateAsync({
        id: editingWebhook.webhook_id,
        params: params as UpdateWebhookParams,
      });
      toast.success('Webhook updated');
      closeDrawer();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update webhook');
    }
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load webhooks'}
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
        <h1 className="text-xl font-semibold text-text-primary">Webhooks</h1>
        <button
          onClick={openCreate}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Plus size={16} />
          New Webhook
        </button>
      </div>

      {isLoading ? (
        <SkeletonTable rows={4} columns={5} />
      ) : !webhooks || webhooks.length === 0 ? (
        <EmptyState
          icon={<Webhook size={40} />}
          title="No webhooks yet"
          description="Create a webhook to receive real-time events when things happen in claude-plane."
          action={
            <button
              onClick={openCreate}
              className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
            >
              <Plus size={16} />
              New Webhook
            </button>
          }
        />
      ) : (
        <WebhooksList
          webhooks={webhooks}
          onEdit={openEdit}
          onViewDeliveries={openDeliveries}
        />
      )}

      {/* Side drawer */}
      {(drawerMode === 'create' || drawerMode === 'edit') && (
        <div className="fixed inset-0 z-40 flex">
          <div className="absolute inset-0 bg-black/50" onClick={closeDrawer} />
          <div className="relative ml-auto h-full w-full max-w-lg bg-bg-secondary border-l border-gray-700 overflow-y-auto flex flex-col">
            <div className="flex items-center justify-between px-6 py-4 border-b border-gray-700">
              <h2 className="text-lg font-semibold text-text-primary">
                {drawerMode === 'create' ? 'New Webhook' : 'Edit Webhook'}
              </h2>
              <button
                onClick={closeDrawer}
                className="text-text-secondary hover:text-text-primary transition-colors text-xl leading-none"
                aria-label="Close"
              >
                ×
              </button>
            </div>
            <div className="flex-1 px-6 py-5">
              <WebhookForm
                initial={drawerMode === 'edit' ? editingWebhook ?? undefined : undefined}
                onSubmit={drawerMode === 'create' ? handleCreate : handleUpdate}
                onCancel={closeDrawer}
                submitting={createWebhook.isPending || updateWebhook.isPending}
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
