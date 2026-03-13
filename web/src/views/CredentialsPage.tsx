import { useState } from 'react';
import { Plus, Lock, AlertCircle, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { useCredentials, useCreateCredential } from '../hooks/useCredentials.ts';
import { CredentialsList } from '../components/credentials/CredentialsList.tsx';
import { CreateCredentialModal } from '../components/credentials/CreateCredentialModal.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';

export function CredentialsPage() {
  const { data: credentials, isLoading, error, refetch } = useCredentials();
  const createCredential = useCreateCredential();
  const [showModal, setShowModal] = useState(false);

  async function handleCreate(name: string, value: string) {
    try {
      await createCredential.mutateAsync({ name, value });
      toast.success('Credential saved');
      setShowModal(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save credential');
    }
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load credentials'}
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
        <div>
          <h1 className="text-xl font-semibold text-text-primary">Credentials</h1>
          <p className="text-sm text-text-secondary mt-0.5">
            Store encrypted secrets that job steps can reference at runtime.
          </p>
        </div>
        <button
          onClick={() => setShowModal(true)}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Plus size={16} />
          New Credential
        </button>
      </div>

      {isLoading ? (
        <SkeletonTable rows={4} columns={4} />
      ) : !credentials || credentials.length === 0 ? (
        <EmptyState
          icon={<Lock size={40} />}
          title="No credentials yet"
          description="Store API keys and tokens here. Values are encrypted at rest and never returned by the API."
          action={
            <button
              onClick={() => setShowModal(true)}
              className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
            >
              <Plus size={16} />
              New Credential
            </button>
          }
        />
      ) : (
        <CredentialsList credentials={credentials} />
      )}

      {showModal && (
        <CreateCredentialModal
          onSubmit={handleCreate}
          onCancel={() => setShowModal(false)}
          submitting={createCredential.isPending}
        />
      )}
    </div>
  );
}
