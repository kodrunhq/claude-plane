import { useState, useCallback, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Copy, Check, ShieldAlert } from 'lucide-react';
import { toast } from 'sonner';
import { copyToClipboard } from '../../lib/clipboard.ts';

interface SecretRevealModalProps {
  open: boolean;
  secret: string;
  onClose: () => void;
}

export function SecretRevealModal({ open, secret, onClose }: SecretRevealModalProps) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open) {
      setCopied(false);
    }
  }, [open]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown);
      return () => document.removeEventListener('keydown', handleKeyDown);
    }
  }, [open, handleKeyDown]);

  async function handleCopy() {
    try {
      await copyToClipboard(secret);
      setCopied(true);
      toast.success('Secret copied to clipboard');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Failed to copy secret');
    }
  }

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" />
      <div role="dialog" aria-modal="true" aria-labelledby="secret-reveal-title" className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
        <div className="flex items-center gap-2 mb-4">
          <ShieldAlert size={20} className="text-status-warning" />
          <h2 id="secret-reveal-title" className="text-lg font-semibold text-text-primary">Webhook Secret</h2>
        </div>

        <div className="flex flex-col gap-4">
          <div className="rounded-md bg-status-warning/10 border border-status-warning/30 px-4 py-3">
            <p className="text-sm text-status-warning font-medium">
              Save your webhook secret -- it won't be shown again.
            </p>
          </div>

          <div>
            <label className="block text-sm text-text-secondary mb-1">Secret</label>
            <div className="flex items-center gap-2">
              <code className="flex-1 min-w-0 block rounded-md bg-bg-tertiary border border-border-primary text-text-primary text-xs px-3 py-2 font-mono break-all select-all">
                {secret}
              </code>
              <button
                type="button"
                onClick={handleCopy}
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
      </div>
    </div>,
    document.body,
  );
}
