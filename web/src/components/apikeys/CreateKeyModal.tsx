import { useState, useCallback, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Copy, Check } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateApiKey } from '../../hooks/useApiKeys.ts';
import { copyToClipboard } from '../../lib/clipboard.ts';

interface CreateKeyModalProps {
  open: boolean;
  onClose: () => void;
}

type ModalState =
  | { stage: 'form' }
  | { stage: 'created'; plaintext: string };

export function CreateKeyModal({ open, onClose }: CreateKeyModalProps) {
  const createApiKey = useCreateApiKey();
  const [name, setName] = useState('');
  const [modalState, setModalState] = useState<ModalState>({ stage: 'form' });
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting form on close
      setName('');
      setModalState({ stage: 'form' });
      setCopied(false);
    }
  }, [open]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape' && modalState.stage === 'form') {
        onClose();
      }
    },
    [onClose, modalState.stage],
  );

  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown);
      return () => document.removeEventListener('keydown', handleKeyDown);
    }
  }, [open, handleKeyDown]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;

    try {
      const result = await createApiKey.mutateAsync({ name: trimmed });
      setModalState({ stage: 'created', plaintext: result.key });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create API key');
    }
  }

  async function handleCopy(plaintext: string) {
    try {
      await copyToClipboard(plaintext);
      setCopied(true);
      toast.success('Key copied to clipboard');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Failed to copy key');
    }
  }

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-black/60"
        onClick={modalState.stage === 'form' ? onClose : undefined}
      />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
        <h2 className="text-lg font-semibold text-text-primary mb-4">Create API Key</h2>

        {modalState.stage === 'form' ? (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div>
              <label className="block text-sm text-text-secondary mb-1">
                Name <span className="text-status-error">*</span>
              </label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. bridge-production"
                autoFocus
                required
                className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30"
              />
            </div>

            <div className="flex justify-end gap-3 mt-2">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={!name.trim() || createApiKey.isPending}
                className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {createApiKey.isPending ? 'Creating...' : 'Create'}
              </button>
            </div>
          </form>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="rounded-md bg-status-warning/10 border border-status-warning/30 px-4 py-3">
              <p className="text-sm text-status-warning font-medium">
                This key will only be shown once. Copy it now.
              </p>
            </div>

            <div>
              <label className="block text-sm text-text-secondary mb-1">API Key</label>
              <div className="flex items-center gap-2">
                <code className="flex-1 min-w-0 block rounded-md bg-bg-tertiary border border-border-primary text-text-primary text-xs px-3 py-2 font-mono break-all select-all">
                  {modalState.plaintext}
                </code>
                <button
                  type="button"
                  onClick={() => handleCopy(modalState.plaintext)}
                  title="Copy to clipboard"
                  className="shrink-0 p-2 rounded-md text-text-secondary hover:text-accent-primary hover:bg-accent-primary/10 transition-colors"
                >
                  {copied ? <Check size={16} className="text-status-success" /> : <Copy size={16} />}
                </button>
              </div>
            </div>

            <div className="flex justify-end mt-2">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
              >
                Done
              </button>
            </div>
          </div>
        )}
      </div>
    </div>,
    document.body,
  );
}
