import { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import type { CreateUserParams } from '../../types/user.ts';

interface CreateUserModalProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (params: CreateUserParams) => Promise<void>;
  submitting: boolean;
}

export function CreateUserModal({ open, onClose, onSubmit, submitting }: CreateUserModalProps) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [role, setRole] = useState<'user' | 'admin'>('user');

  function reset() {
    setEmail('');
    setPassword('');
    setDisplayName('');
    setRole('user');
  }

  function handleClose() {
    reset();
    onClose();
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await onSubmit({ email, password, display_name: displayName, role });
      reset();
    } catch {
      // Parent handles error display; preserve form state so user can retry
    }
  }

  useEffect(() => {
    if (!open) return;
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose();
    };
    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- handleClose is stable across renders when open=true
  }, [open]);

  if (!open) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/60" onClick={handleClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4">
        <div className="flex items-center justify-between px-6 py-4 border-b border-border-primary">
          <h2 className="text-lg font-semibold text-text-primary">New User</h2>
          <button
            onClick={handleClose}
            className="text-text-secondary hover:text-text-primary transition-colors text-xl leading-none"
            aria-label="Close"
          >
            &times;
          </button>
        </div>

        <form onSubmit={handleSubmit} className="px-6 py-5 space-y-4">
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Email <span className="text-status-error">*</span>
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              placeholder="user@example.com"
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Password <span className="text-status-error">*</span>
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              minLength={8}
              placeholder="Min 8 characters"
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Display Name
            </label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="Optional"
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Role
            </label>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value as 'user' | 'admin')}
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary focus:outline-none focus:border-accent-primary"
            >
              <option value="user">user</option>
              <option value="admin">admin</option>
            </select>
          </div>

          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={handleClose}
              className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50"
            >
              {submitting ? 'Creating...' : 'Create User'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
