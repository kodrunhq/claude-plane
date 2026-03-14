import { useState } from 'react';
import { Plus, Plug, RefreshCw, AlertCircle } from 'lucide-react';
import { toast } from 'sonner';
import { useConnectors, useDeleteConnector, useRestartBridge } from '../hooks/useBridge.ts';
import { ConnectorCard } from '../components/connectors/ConnectorCard.tsx';
import { AddConnectorModal } from '../components/connectors/AddConnectorModal.tsx';
import { TelegramForm } from '../components/connectors/TelegramForm.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import type { BridgeConnector } from '../types/connector.ts';

export function ConnectorsPage() {
  const { data: connectors, isLoading } = useConnectors();
  const deleteConnector = useDeleteConnector();
  const restartBridge = useRestartBridge();

  const [showAddModal, setShowAddModal] = useState(false);
  const [editingConnector, setEditingConnector] = useState<BridgeConnector | null>(null);
  const [showTelegramForm, setShowTelegramForm] = useState(false);
  const [configChanged, setConfigChanged] = useState(false);
  const [deletingConnector, setDeletingConnector] = useState<BridgeConnector | null>(null);

  function handleSelectType(type: string) {
    setShowAddModal(false);
    if (type === 'telegram') {
      setEditingConnector(null);
      setShowTelegramForm(true);
    }
  }

  function handleEdit(connector: BridgeConnector) {
    setEditingConnector(connector);
    if (connector.connector_type === 'telegram') {
      setShowTelegramForm(true);
    }
  }

  function handleFormClose() {
    setShowTelegramForm(false);
    setEditingConnector(null);
    setConfigChanged(true);
  }

  async function handleDelete() {
    if (!deletingConnector) return;
    try {
      await deleteConnector.mutateAsync(deletingConnector.connector_id);
      toast.success('Connector deleted');
      setConfigChanged(true);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete connector');
    }
    setDeletingConnector(null);
  }

  async function handleRestart() {
    try {
      await restartBridge.mutateAsync();
      toast.success('Bridge restart requested');
      setConfigChanged(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to restart bridge');
    }
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Plug size={18} className="text-text-secondary" />
          <h1 className="text-xl font-semibold text-text-primary">Connectors</h1>
        </div>
        <button
          onClick={() => setShowAddModal(true)}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all hover:shadow-[0_0_20px_rgba(59,130,246,0.3)]"
        >
          <Plus size={16} />
          Add Connector
        </button>
      </div>

      {/* Config changed banner */}
      {configChanged && (
        <div className="flex items-center justify-between gap-4 rounded-lg border border-status-warning/40 bg-status-warning/10 px-4 py-3">
          <div className="flex items-center gap-2 text-sm text-status-warning">
            <AlertCircle size={16} className="shrink-0" />
            Configuration saved. Restart bridges to apply changes.
          </div>
          <button
            onClick={handleRestart}
            disabled={restartBridge.isPending}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-md bg-status-warning/20 hover:bg-status-warning/30 text-status-warning border border-status-warning/40 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <RefreshCw size={13} className={restartBridge.isPending ? 'animate-spin' : ''} />
            {restartBridge.isPending ? 'Restarting...' : 'Apply & Restart'}
          </button>
        </div>
      )}

      {/* Content */}
      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 3 }, (_, i) => (
            <div
              key={i}
              className="bg-bg-secondary border border-border-primary rounded-lg p-4 animate-pulse h-28"
            />
          ))}
        </div>
      ) : !connectors || connectors.length === 0 ? (
        <div className="bg-bg-secondary rounded-lg border border-border-primary p-10 text-center">
          <Plug size={32} className="mx-auto text-text-secondary/30 mb-3" />
          <p className="text-sm text-text-secondary mb-3">
            No connectors configured. Add a connector to start receiving notifications.
          </p>
          <button
            onClick={() => setShowAddModal(true)}
            className="inline-flex items-center gap-1.5 text-sm text-accent-primary hover:text-accent-primary/80 transition-colors"
          >
            <Plus size={14} />
            Add your first connector
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {connectors.map((connector) => (
            <ConnectorCard
              key={connector.connector_id}
              connector={connector}
              onEdit={handleEdit}
              onDelete={setDeletingConnector}
            />
          ))}
        </div>
      )}

      {/* Add connector type picker */}
      <AddConnectorModal
        open={showAddModal}
        onClose={() => setShowAddModal(false)}
        onSelectType={handleSelectType}
      />

      {/* Telegram create/edit form */}
      {showTelegramForm && (
        <TelegramForm connector={editingConnector ?? undefined} onClose={handleFormClose} />
      )}

      {/* Delete confirmation */}
      <ConfirmDialog
        open={deletingConnector !== null}
        title="Delete Connector"
        message={`Are you sure you want to delete "${deletingConnector?.name}"? This cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeletingConnector(null)}
      />
    </div>
  );
}
