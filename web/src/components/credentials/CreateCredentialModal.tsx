import { useState, useEffect } from 'react';
import { Eye, EyeOff } from 'lucide-react';

interface CreateCredentialModalProps {
  onSubmit: (name: string, value: string) => Promise<void>;
  onCancel: () => void;
  submitting: boolean;
}

export function CreateCredentialModal({ onSubmit, onCancel, submitting }: CreateCredentialModalProps) {
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [showValue, setShowValue] = useState(false);

  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel();
    };
    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [onCancel]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !value.trim()) return;
    await onSubmit(name.trim(), value.trim());
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/60" onClick={onCancel} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4">
        <div className="flex items-center justify-between px-6 py-4 border-b border-border-primary">
          <h2 className="text-lg font-semibold text-text-primary">New Credential</h2>
          <button
            onClick={onCancel}
            className="text-text-secondary hover:text-text-primary transition-colors text-xl leading-none"
            aria-label="Close"
          >
            ×
          </button>
        </div>

        <form onSubmit={handleSubmit} className="px-6 py-5 space-y-4">
          <div className="space-y-1.5">
            <label htmlFor="cred-name" className="block text-sm font-medium text-text-secondary">
              Name
            </label>
            <input
              id="cred-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. GITHUB_TOKEN"
              className="w-full px-3 py-2 text-sm rounded-md bg-bg-tertiary border border-gray-600 text-text-primary placeholder:text-text-secondary/60 focus:outline-none focus:border-accent-primary transition-colors"
              required
              autoFocus
            />
          </div>

          <div className="space-y-1.5">
            <label htmlFor="cred-value" className="block text-sm font-medium text-text-secondary">
              Secret Value
            </label>
            <div className="relative">
              <input
                id="cred-value"
                type={showValue ? 'text' : 'password'}
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder="Paste your secret here"
                className="w-full px-3 py-2 pr-10 text-sm rounded-md bg-bg-tertiary border border-gray-600 text-text-primary placeholder:text-text-secondary/60 focus:outline-none focus:border-accent-primary transition-colors font-mono"
                required
              />
              <button
                type="button"
                onClick={() => setShowValue((v) => !v)}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary transition-colors"
                aria-label={showValue ? 'Hide value' : 'Show value'}
              >
                {showValue ? <EyeOff size={15} /> : <Eye size={15} />}
              </button>
            </div>
            <p className="text-xs text-text-secondary">
              The value will be encrypted at rest. It cannot be retrieved after saving.
            </p>
          </div>

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onCancel}
              className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting || !name.trim() || !value.trim()}
              className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? 'Saving...' : 'Save Credential'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
