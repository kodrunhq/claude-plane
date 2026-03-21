import { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import type { User, UpdateUserParams } from '../../types/user.ts';

interface EditUserModalProps {
  open: boolean;
  user: User | null;
  onClose: () => void;
  onSubmit: (id: string, params: UpdateUserParams) => Promise<void>;
  submitting: boolean;
}

export function EditUserModal({ open, user, onClose, onSubmit, submitting }: EditUserModalProps) {
  useEffect(() => {
    if (!open) return;
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [open, onClose]);

  if (!open || !user) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4">
        <div className="flex items-center justify-between px-6 py-4 border-b border-border-primary">
          <h2 className="text-lg font-semibold text-text-primary">Edit User</h2>
          <button
            onClick={onClose}
            className="text-text-secondary hover:text-text-primary transition-colors text-xl leading-none"
            aria-label="Close"
          >
            &times;
          </button>
        </div>
        <EditUserForm key={user.user_id} user={user} onClose={onClose} onSubmit={onSubmit} submitting={submitting} />
      </div>
    </div>,
    document.body,
  );
}

function EditUserForm({
  user,
  onClose,
  onSubmit,
  submitting,
}: {
  user: User;
  onClose: () => void;
  onSubmit: (id: string, params: UpdateUserParams) => Promise<void>;
  submitting: boolean;
}) {
  const [displayName, setDisplayName] = useState(user.display_name ?? '');
  const [role, setRole] = useState<'user' | 'admin'>((user.role as 'user' | 'admin') ?? 'user');

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await onSubmit(user.user_id, { display_name: displayName, role });
  }

  return (
    <form onSubmit={handleSubmit} className="px-6 py-5 space-y-4">
      <div>
        <label className="block text-sm font-medium text-text-secondary mb-1.5">
          Email
        </label>
        <input
          type="text"
          value={user.email}
          disabled
          className="w-full px-3 py-2 text-sm bg-bg-tertiary/50 border border-border-primary rounded-md text-text-secondary cursor-not-allowed"
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
          onClick={onClose}
          className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting}
          className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50"
        >
          {submitting ? 'Saving...' : 'Save Changes'}
        </button>
      </div>
    </form>
  );
}
