import { useState, useMemo } from 'react';
import { Plus, Trash2, Key } from 'lucide-react';
import { toast } from 'sonner';
import { useApiKeys, useDeleteApiKey } from '../hooks/useApiKeys.ts';
import { CreateKeyModal } from '../components/apikeys/CreateKeyModal.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { TimeAgo } from '../components/shared/TimeAgo.tsx';
import type { APIKey } from '../types/apikey.ts';

function truncateKeyID(key: APIKey): string {
  // Truncate the key_id UUID for compact table display.
  const id = key.key_id;
  return id.length > 16 ? `${id.slice(0, 16)}…` : id;
}

export function ApiKeysPage() {
  const { data: apiKeys, isLoading } = useApiKeys();
  const deleteApiKey = useDeleteApiKey();

  const [createOpen, setCreateOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const sortedKeys = useMemo(
    () => [...(apiKeys ?? [])].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [apiKeys],
  );

  async function handleDelete() {
    if (!deleteId) return;
    try {
      await deleteApiKey.mutateAsync(deleteId);
      toast.success('API key deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete API key');
    }
    setDeleteId(null);
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Key size={18} className="text-text-secondary" />
          <h1 className="text-xl font-semibold text-text-primary">API Keys</h1>
        </div>
        <button
          onClick={() => setCreateOpen(true)}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all hover:shadow-[0_0_20px_rgba(59,130,246,0.3)]"
        >
          <Plus size={16} />
          Create API Key
        </button>
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div
              key={i}
              className="bg-bg-secondary rounded-lg border border-border-primary p-4 animate-pulse"
            >
              <div className="h-4 bg-bg-tertiary rounded w-1/4 mb-2" />
              <div className="h-3 bg-bg-tertiary rounded w-1/2" />
            </div>
          ))}
        </div>
      ) : sortedKeys.length === 0 ? (
        <div className="bg-bg-secondary rounded-lg border border-border-primary p-8 text-center">
          <p className="text-sm text-text-secondary mb-3">
            No API keys yet. Create one to authenticate bridge connections.
          </p>
          <button
            onClick={() => setCreateOpen(true)}
            className="inline-flex items-center gap-1.5 text-sm text-accent-primary hover:text-accent-primary/80 transition-colors"
          >
            <Plus size={14} />
            Create your first API key
          </button>
        </div>
      ) : (
        <div className="bg-bg-secondary rounded-lg border border-border-primary overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border-primary">
                <th className="text-left px-4 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">
                  Name
                </th>
                <th className="text-left px-4 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">
                  Key ID
                </th>
                <th className="text-left px-4 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">
                  Created
                </th>
                <th className="text-left px-4 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">
                  Last Used
                </th>
                <th className="text-left px-4 py-3 text-xs font-medium text-text-secondary uppercase tracking-wider">
                  Expires
                </th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border-primary">
              {sortedKeys.map((key) => (
                <tr
                  key={key.key_id}
                  className="hover:bg-accent-primary/5 transition-colors"
                >
                  <td className="px-4 py-3 text-text-primary font-medium">{key.name}</td>
                  <td className="px-4 py-3">
                    <code className="text-xs font-mono text-text-secondary bg-bg-tertiary rounded px-1.5 py-0.5">
                      {truncateKeyID(key)}
                    </code>
                  </td>
                  <td className="px-4 py-3 text-text-secondary text-xs">
                    <TimeAgo date={key.created_at} />
                  </td>
                  <td className="px-4 py-3 text-text-secondary text-xs">
                    {key.last_used_at ? (
                      <TimeAgo date={key.last_used_at} />
                    ) : (
                      <span className="text-text-secondary/40">Never</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-text-secondary text-xs">
                    {key.expires_at ? (
                      <TimeAgo date={key.expires_at} />
                    ) : (
                      <span className="text-text-secondary/40">Never</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={() => setDeleteId(key.key_id)}
                      className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
                      title="Delete key"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <CreateKeyModal open={createOpen} onClose={() => setCreateOpen(false)} />

      <ConfirmDialog
        open={deleteId !== null}
        title="Delete API Key"
        message="Are you sure you want to delete this API key? Any clients using it will lose access immediately."
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeleteId(null)}
      />
    </div>
  );
}
