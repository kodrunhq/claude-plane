import { Link } from 'react-router';
import { KeyRound, AlertCircle, RefreshCw } from 'lucide-react';
import { useProvisioningTokens } from '../hooks/useProvisioning.ts';
import { TokenGenerator } from '../components/provisioning/TokenGenerator.tsx';
import { TokensList } from '../components/provisioning/TokensList.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';

export function ProvisioningPage() {
  const { data: tokens, isLoading, error, refetch } = useProvisioningTokens();

  if (error) {
    return (
      <div className="p-4 md:p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load provisioning tokens'}
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
    <div className="p-4 md:p-6 space-y-6">
      <div>
        <h1 className="text-xl font-semibold text-text-primary">Provisioning</h1>
        <p className="text-sm text-text-secondary mt-0.5">
          Generate one-time install tokens for new agent machines.{' '}
          <Link to="/docs/getting-started" className="text-accent-primary hover:underline">
            How to install an agent
          </Link>
        </p>
      </div>

      <TokenGenerator />

      <div className="space-y-3">
        <h2 className="text-sm font-semibold text-text-primary">All Tokens</h2>

        {isLoading ? (
          <SkeletonTable rows={3} columns={7} />
        ) : !tokens || tokens.length === 0 ? (
          <EmptyState
            icon={<KeyRound size={40} />}
            title="No provisioning tokens yet"
            description="Generate a token above to provision a new agent machine."
          />
        ) : (
          <TokensList tokens={tokens} />
        )}
      </div>
    </div>
  );
}
