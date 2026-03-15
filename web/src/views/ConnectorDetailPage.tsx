import { useState, useCallback, useMemo, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router';
import { ArrowLeft, Plus, Pencil, Trash2, RefreshCw, AlertCircle, Github, MessageCircle } from 'lucide-react';
import { toast } from 'sonner';
import { useConnector, useUpdateConnector, useDeleteConnector, useRestartBridge } from '../hooks/useBridge.ts';
import { WatchEditor } from '../components/connectors/WatchEditor.tsx';
import type { WatchData } from '../components/connectors/WatchEditor.tsx';
import type { TriggerFilters } from '../components/connectors/TriggerConfig.tsx';
import { createDefaultWatch } from '../components/connectors/watchDefaults.ts';
import { GithubForm } from '../components/connectors/GithubForm.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';

function buildConfigJson(watches: WatchData[]): string {
  const serialized = watches.map((w) => ({
    repo: w.repo,
    template: w.template,
    machine_id: w.machine_id,
    poll_interval: w.poll_interval,
    triggers: {
      pull_request_opened: w.triggers.pull_request_opened.enabled
        ? { filters: w.triggers.pull_request_opened.filters }
        : null,
      check_run_completed: w.triggers.check_run_completed.enabled
        ? { filters: w.triggers.check_run_completed.filters }
        : null,
      issue_labeled: w.triggers.issue_labeled.enabled
        ? { filters: w.triggers.issue_labeled.filters }
        : null,
      issue_comment: w.triggers.issue_comment.enabled
        ? { filters: w.triggers.issue_comment.filters }
        : null,
      pull_request_comment: w.triggers.pull_request_comment.enabled
        ? { filters: w.triggers.pull_request_comment.filters }
        : null,
      pull_request_review: w.triggers.pull_request_review.enabled
        ? { filters: w.triggers.pull_request_review.filters }
        : null,
      release_published: w.triggers.release_published.enabled
        ? { filters: w.triggers.release_published.filters }
        : null,
    },
  }));
  return JSON.stringify({ watches: serialized });
}

function hydrateWatches(raw: unknown): WatchData[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((w) => {
    const item = w as Record<string, unknown>;
    const triggers = (item.triggers ?? {}) as Record<string, unknown>;

    function hydrateT(key: string): { enabled: boolean; filters: TriggerFilters } {
      const t = triggers[key];
      if (!t || typeof t !== 'object') return { enabled: false, filters: {} };
      return { enabled: true, filters: ((t as Record<string, unknown>).filters ?? {}) as TriggerFilters };
    }

    return {
      id: `watch-hydrated-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      repo: String(item.repo ?? ''),
      template: String(item.template ?? ''),
      machine_id: String(item.machine_id ?? ''),
      poll_interval: String(item.poll_interval ?? '60s'),
      triggers: {
        pull_request_opened: hydrateT('pull_request_opened'),
        check_run_completed: hydrateT('check_run_completed'),
        issue_labeled: hydrateT('issue_labeled'),
        issue_comment: hydrateT('issue_comment'),
        pull_request_comment: hydrateT('pull_request_comment'),
        pull_request_review: hydrateT('pull_request_review'),
        release_published: hydrateT('release_published'),
      },
    };
  });
}

function ConnectorIcon({ type }: { type: string }) {
  if (type === 'github') {
    return <Github size={20} className="text-text-secondary" />;
  }
  return <MessageCircle size={20} className="text-accent-primary" />;
}

function typeBadge(type: string): string {
  const labels: Record<string, string> = {
    telegram: 'Telegram',
    github: 'GitHub',
  };
  return labels[type] ?? type;
}

export function ConnectorDetailPage() {
  const { connectorId } = useParams<{ connectorId: string }>();
  const navigate = useNavigate();
  const { data: connector, isLoading } = useConnector(connectorId);
  const updateConnector = useUpdateConnector();
  const deleteConnectorMut = useDeleteConnector();
  const restartBridge = useRestartBridge();

  // Derive watches from connector config; reset local edits when server data changes
  const serverWatches = useMemo(() => {
    if (!connector?.config) return [];
    try {
      const parsed = JSON.parse(connector.config) as { watches?: unknown };
      return hydrateWatches(parsed.watches ?? []);
    } catch {
      return [];
    }
  }, [connector?.config]);

  const [localWatches, setLocalWatches] = useState<WatchData[] | null>(null);
  const [dirtyState, setDirtyState] = useState(false);

  // When server data changes, clear local overrides
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting local form state when server config changes
    setLocalWatches(null);
    setDirtyState(false);
  }, [connector?.config]);

  const watches = localWatches ?? serverWatches;
  const dirty = dirtyState;
  const [configChanged, setConfigChanged] = useState(false);
  const [showEditForm, setShowEditForm] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  function setWatches(updater: WatchData[] | ((prev: WatchData[]) => WatchData[])) {
    if (typeof updater === 'function') {
      setLocalWatches((prev) => updater(prev ?? serverWatches));
    } else {
      setLocalWatches(updater);
    }
  }

  const addWatch = useCallback(() => {
    setLocalWatches((prev) => [...(prev ?? serverWatches), createDefaultWatch()]);
    setDirtyState(true);
  }, [serverWatches]);

  function updateWatch(index: number, updated: WatchData) {
    setWatches((prev) => prev.map((w, i) => (i === index ? updated : w)));
    setDirtyState(true);
  }

  function removeWatch(index: number) {
    setWatches((prev) => prev.filter((_, i) => i !== index));
    setDirtyState(true);
  }

  async function handleSaveWatches() {
    if (!connector || !connectorId) return;
    const config = buildConfigJson(watches);
    try {
      await updateConnector.mutateAsync({
        id: connectorId,
        params: {
          connector_type: connector.connector_type,
          name: connector.name,
          config,
        },
      });
      toast.success('Watches saved');
      setDirtyState(false);
      setConfigChanged(true);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save watches');
    }
  }

  async function handleDelete() {
    if (!connectorId) return;
    try {
      await deleteConnectorMut.mutateAsync(connectorId);
      toast.success('Connector deleted');
      navigate('/connectors');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete connector');
    }
    setShowDeleteConfirm(false);
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

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-48 bg-bg-secondary rounded" />
          <div className="h-32 bg-bg-secondary rounded-lg" />
        </div>
      </div>
    );
  }

  if (!connector) {
    return (
      <div className="p-6">
        <p className="text-text-secondary">Connector not found.</p>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      {/* Back link */}
      <button
        onClick={() => navigate('/connectors')}
        className="flex items-center gap-1.5 text-sm text-text-secondary hover:text-text-primary transition-colors"
      >
        <ArrowLeft size={14} />
        Back to Connectors
      </button>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <ConnectorIcon type={connector.connector_type} />
          <div>
            <h1 className="text-xl font-semibold text-text-primary">{connector.name}</h1>
            <span className="inline-block mt-0.5 text-xs font-mono bg-bg-tertiary text-text-secondary border border-border-primary rounded px-1.5 py-0.5">
              {typeBadge(connector.connector_type)}
            </span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {connector.connector_type === 'github' && (
            <button
              onClick={() => setShowEditForm(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              <Pencil size={14} />
              Edit
            </button>
          )}
          <button
            onClick={() => setShowDeleteConfirm(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md text-text-secondary hover:text-status-error bg-bg-tertiary hover:bg-status-error/10 transition-colors"
          >
            <Trash2 size={14} />
            Delete
          </button>
        </div>
      </div>

      {/* Status */}
      <div className="flex items-center gap-1.5 text-sm">
        <span
          className={`inline-block h-2 w-2 rounded-full ${
            connector.enabled ? 'bg-status-success' : 'bg-text-secondary/40'
          }`}
        />
        <span className={connector.enabled ? 'text-status-success' : 'text-text-secondary/60'}>
          {connector.enabled ? 'Enabled' : 'Disabled'}
        </span>
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

      {/* Watches section -- GitHub only */}
      {connector.connector_type === 'github' && (
        <div className="bg-bg-secondary border border-border-primary rounded-lg">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border-primary">
            <h2 className="text-sm font-semibold text-text-primary">Watches</h2>
            <button
              type="button"
              onClick={addWatch}
              className="flex items-center gap-1 text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
            >
              <Plus size={13} />
              Add Watch
            </button>
          </div>

          <div className="p-4">
            {watches.length === 0 ? (
              <p className="text-xs text-text-secondary/50 italic text-center py-4">
                No watches configured. Add a watch to monitor a repository.
              </p>
            ) : (
              <div className="flex flex-col gap-3">
                {watches.map((watch, i) => (
                  <WatchEditor
                    key={watch.id}
                    index={i}
                    watch={watch}
                    onChange={(updated) => updateWatch(i, updated)}
                    onRemove={() => removeWatch(i)}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Save watches button */}
          {dirty && (
            <div className="flex justify-end px-4 py-3 border-t border-border-primary">
              <button
                onClick={handleSaveWatches}
                disabled={updateConnector.isPending}
                className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {updateConnector.isPending ? 'Saving...' : 'Save Watches'}
              </button>
            </div>
          )}
        </div>
      )}

      {/* Edit form modal -- GitHub only */}
      {showEditForm && connector.connector_type === 'github' && (
        <GithubForm
          connector={connector}
          onClose={() => setShowEditForm(false)}
          onSaved={() => {
            setShowEditForm(false);
            setConfigChanged(true);
          }}
        />
      )}

      {/* Delete confirmation */}
      <ConfirmDialog
        open={showDeleteConfirm}
        title="Delete Connector"
        message={`Are you sure you want to delete "${connector.name}"? This cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
  );
}
