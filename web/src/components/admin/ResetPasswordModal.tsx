import { useState } from 'react';
import { createPortal } from 'react-dom';
import type { User } from '../../types/user.ts';

interface ResetPasswordModalProps {
  open: boolean;
  user: User | null;
  onClose: () => void;
  onSubmit: (userId: string, newPassword: string) => Promise<void>;
  submitting: boolean;
}

export function ResetPasswordModal({ open, user, onClose, onSubmit, submitting }: ResetPasswordModalProps) {
  if (!open || !user) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div role="dialog" aria-modal="true" aria-labelledby="reset-password-title" className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-md w-full mx-4">
        <div className="flex items-center justify-between px-6 py-4 border-b border-border-primary">
          <h2 id="reset-password-title" className="text-lg font-semibold text-text-primary">Reset Password</h2>
          <button
            onClick={onClose}
            className="text-text-secondary hover:text-text-primary transition-colors text-xl leading-none"
            aria-label="Close"
          >
            &times;
          </button>
        </div>
        <ResetPasswordForm
          key={user.user_id}
          user={user}
          onSubmit={onSubmit}
          submitting={submitting}
        />
      </div>
    </div>,
    document.body,
  );
}

function ResetPasswordForm({
  user,
  onSubmit,
  submitting,
}: {
  user: User;
  onSubmit: (userId: string, newPassword: string) => Promise<void>;
  submitting: boolean;
}) {
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    try {
      await onSubmit(user.user_id, newPassword);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset password');
    }
  }

  return (
    <form onSubmit={handleSubmit} className="px-6 py-5 space-y-4">
      <p className="text-sm text-text-secondary">
        Set a new password for <span className="font-medium text-text-primary">{user.email}</span>.
      </p>

      <div>
        <label htmlFor="reset-new-password" className="block text-sm font-medium text-text-secondary mb-1.5">
          New Password
        </label>
        <input
          id="reset-new-password"
          type="password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          required
          minLength={8}
          placeholder="Minimum 8 characters"
          className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
        />
      </div>

      <div>
        <label htmlFor="reset-confirm-password" className="block text-sm font-medium text-text-secondary mb-1.5">
          Confirm Password
        </label>
        <input
          id="reset-confirm-password"
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          required
          minLength={8}
          className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
        />
      </div>

      {error && <p className="text-sm text-status-error">{error}</p>}

      <div className="flex justify-end gap-3 pt-2">
        <button
          type="submit"
          disabled={submitting || !newPassword || !confirmPassword}
          className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50"
        >
          {submitting ? 'Resetting...' : 'Reset Password'}
        </button>
      </div>
    </form>
  );
}
