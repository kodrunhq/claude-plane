import { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router';
import { X } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateConnector, useUpdateConnector } from '../../hooks/useBridge.ts';
import type { BridgeConnector } from '../../types/connector.ts';

interface GithubFormProps {
  connector?: BridgeConnector;
  onClose: () => void;
  onSaved?: () => void;
}

const inputClass =
  'w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30';

export function GithubForm({ connector, onClose, onSaved }: GithubFormProps) {
  const isEdit = connector !== undefined;
  const navigate = useNavigate();

  const [name, setName] = useState(connector?.name ?? '');
  const [token, setToken] = useState('');

  const createConnector = useCreateConnector();
  const updateConnector = useUpdateConnector();
  const isPending = createConnector.isPending || updateConnector.isPending;

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    const configSecret =
      token.trim() ? JSON.stringify({ token: token.trim() }) : undefined;

    try {
      if (isEdit) {
        await updateConnector.mutateAsync({
          id: connector.connector_id,
          params: {
            connector_type: 'github',
            name: name.trim(),
            config: connector.config,
            ...(configSecret !== undefined ? { config_secret: configSecret } : {}),
          },
        });
        toast.success('Connector updated');

        if (onSaved) {
          onSaved();
        } else {
          onClose();
        }
      } else {
        const result = await createConnector.mutateAsync({
          connector_type: 'github',
          name: name.trim(),
          config: JSON.stringify({ watches: [] }),
          ...(configSecret !== undefined ? { config_secret: configSecret } : {}),
        });
        toast.success('Connector created');
        onClose();
        navigate(`/connectors/${result.connector_id}`);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save connector');
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold text-text-primary">
            {isEdit ? 'Edit GitHub Connector' : 'New GitHub Connector'}
          </h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          {/* Connector name */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Connector name <span className="text-status-error">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. my-github"
              required
              autoFocus
              className={inputClass}
            />
          </div>

          {/* GitHub token */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              GitHub token{' '}
              {!isEdit && <span className="text-status-error">*</span>}
              {isEdit && (
                <span className="text-text-secondary/50 ml-1">
                  (leave blank to keep existing)
                </span>
              )}
            </label>
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="ghp_..."
              required={!isEdit}
              className={inputClass}
            />
            <p className="mt-1 text-xs text-text-secondary/50">
              Requires <code className="font-mono">repo</code> scope for PR, check run, and
              issue access.
            </p>
          </div>

          {/* Actions */}
          <div className="flex justify-end gap-3 mt-1">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isPending}
              className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isPending ? 'Saving...' : isEdit ? 'Save changes' : 'Create connector'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
