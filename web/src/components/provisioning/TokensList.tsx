import { useState } from 'react';
import { Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { ConfirmDialog } from '../shared/ConfirmDialog.tsx';
import { useRevokeProvisioningToken } from '../../hooks/useProvisioning.ts';
import { getTokenStatus, type ProvisioningToken } from '../../types/provisioning.ts';
import { formatDistanceToNow, formatDistanceToNowStrict } from 'date-fns';
import { truncateId } from '../../lib/format.ts';

interface TokensListProps {
  tokens: ProvisioningToken[];
}

interface TokenStatusBadgeProps {
  token: ProvisioningToken;
}

function TokenStatusBadge({ token }: TokenStatusBadgeProps) {
  const status = getTokenStatus(token);

  const styles: Record<string, string> = {
    active: 'bg-status-success/15 text-status-success',
    expired: 'bg-status-warning/15 text-status-warning',
    redeemed: 'bg-accent-primary/15 text-accent-primary',
  };

  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 text-xs rounded-full font-medium ${styles[status]}`}
    >
      {status}
    </span>
  );
}

function ExpiryCell({ token }: { token: ProvisioningToken }) {
  const status = getTokenStatus(token);

  if (status === 'redeemed' && token.redeemed_at) {
    return (
      <span className="text-xs text-text-secondary">
        redeemed {formatDistanceToNow(new Date(token.redeemed_at), { addSuffix: true })}
      </span>
    );
  }

  if (status === 'expired') {
    return (
      <span className="text-xs text-text-secondary">
        expired {formatDistanceToNow(new Date(token.expires_at), { addSuffix: true })}
      </span>
    );
  }

  return (
    <span className="text-xs text-status-success">
      expires in {formatDistanceToNowStrict(new Date(token.expires_at))}
    </span>
  );
}

interface TokenRowProps {
  token: ProvisioningToken;
  onRevokeRequest: (token: ProvisioningToken) => void;
}

function TokenRow({ token, onRevokeRequest }: TokenRowProps) {
  const status = getTokenStatus(token);
  const canRevoke = status === 'active';

  return (
    <tr className="border-t border-gray-800 hover:bg-bg-tertiary/20 transition-colors">
      <td className="px-4 py-3">
        <span className="text-sm font-mono text-text-primary">{token.machine_id}</span>
      </td>
      <td className="px-4 py-3">
        <TokenStatusBadge token={token} />
      </td>
      <td className="px-4 py-3 text-xs font-mono text-text-secondary">
        {token.target_os}/{token.target_arch}
      </td>
      <td className="px-4 py-3">
        <ExpiryCell token={token} />
      </td>
      <td className="px-4 py-3 text-xs text-text-secondary font-mono">
        {truncateId(token.token, 12)}
      </td>
      <td className="px-4 py-3 text-xs text-text-secondary">{token.created_by}</td>
      <td className="px-4 py-3">
        <div className="flex items-center justify-end">
          <button
            onClick={() => onRevokeRequest(token)}
            disabled={!canRevoke}
            className="p-1.5 rounded text-text-secondary hover:text-status-error hover:bg-bg-tertiary transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
            title={canRevoke ? 'Revoke token' : 'Cannot revoke — token is not active'}
          >
            <Trash2 size={15} />
          </button>
        </div>
      </td>
    </tr>
  );
}

export function TokensList({ tokens }: TokensListProps) {
  const [pendingRevoke, setPendingRevoke] = useState<ProvisioningToken | null>(null);
  const revokeToken = useRevokeProvisioningToken();

  async function handleConfirmRevoke() {
    if (!pendingRevoke) return;
    try {
      await revokeToken.mutateAsync(pendingRevoke.token);
      toast.success('Token revoked');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke token');
    } finally {
      setPendingRevoke(null);
    }
  }

  return (
    <>
      <div className="overflow-hidden rounded-lg border border-gray-700">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="bg-bg-secondary">
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Machine ID
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Status
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Platform
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Expiry
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Token
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Created By
              </th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {tokens.map((token) => (
              <TokenRow
                key={token.token}
                token={token}
                onRevokeRequest={setPendingRevoke}
              />
            ))}
          </tbody>
        </table>
      </div>

      <ConfirmDialog
        open={!!pendingRevoke}
        title="Revoke token"
        message={`Revoke the provisioning token for "${pendingRevoke?.machine_id}"? The install command will stop working immediately.`}
        confirmLabel="Revoke"
        variant="danger"
        onConfirm={handleConfirmRevoke}
        onCancel={() => setPendingRevoke(null)}
      />
    </>
  );
}
