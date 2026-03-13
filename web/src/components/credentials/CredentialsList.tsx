import { useState } from 'react';
import { Trash2 } from 'lucide-react';
import { ConfirmDialog } from '../shared/ConfirmDialog.tsx';
import { formatTimeAgo, truncateId } from '../../lib/format.ts';
import { useDeleteCredential } from '../../hooks/useCredentials.ts';
import { toast } from 'sonner';
import type { Credential } from '../../types/credential.ts';

interface CredentialsListProps {
  credentials: Credential[];
}

interface CredentialRowProps {
  credential: Credential;
  onDeleteRequest: (credential: Credential) => void;
}

function CredentialRow({ credential, onDeleteRequest }: CredentialRowProps) {
  return (
    <tr className="border-t border-gray-800 hover:bg-bg-tertiary/20 transition-colors">
      <td className="px-4 py-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium text-text-primary">{credential.name}</span>
          <span className="text-xs text-text-secondary font-mono">{truncateId(credential.credential_id)}</span>
        </div>
      </td>
      <td className="px-4 py-3">
        <span className="text-xs text-text-secondary font-mono bg-bg-tertiary px-2 py-0.5 rounded">
          ••••••••
        </span>
      </td>
      <td className="px-4 py-3 text-sm text-text-secondary">
        {formatTimeAgo(credential.created_at)}
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1 justify-end">
          <button
            onClick={() => onDeleteRequest(credential)}
            className="p-1.5 rounded text-text-secondary hover:text-status-error hover:bg-bg-tertiary transition-colors"
            title="Delete credential"
          >
            <Trash2 size={15} />
          </button>
        </div>
      </td>
    </tr>
  );
}

export function CredentialsList({ credentials }: CredentialsListProps) {
  const [pendingDelete, setPendingDelete] = useState<Credential | null>(null);
  const deleteCredential = useDeleteCredential();

  async function handleConfirmDelete() {
    if (!pendingDelete) return;
    try {
      await deleteCredential.mutateAsync(pendingDelete.credential_id);
      toast.success('Credential deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete credential');
    } finally {
      setPendingDelete(null);
    }
  }

  return (
    <>
      <div className="overflow-hidden rounded-lg border border-gray-700">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="bg-bg-secondary">
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Name
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Value
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Created
              </th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {credentials.map((credential) => (
              <CredentialRow
                key={credential.credential_id}
                credential={credential}
                onDeleteRequest={setPendingDelete}
              />
            ))}
          </tbody>
        </table>
      </div>

      <ConfirmDialog
        open={!!pendingDelete}
        title="Delete credential"
        message={`Are you sure you want to delete "${pendingDelete?.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleConfirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </>
  );
}
