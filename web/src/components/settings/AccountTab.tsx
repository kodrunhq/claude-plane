import { useState, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { useAuthStore } from '../../stores/auth.ts';
import { useChangePassword, useUpdateProfile } from '../../hooks/useUsers.ts';

export function AccountTab() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const changePassword = useChangePassword();
  const updateProfile = useUpdateProfile();

  const prevDisplayNameRef = useRef(user?.displayName);
  const [displayName, setDisplayName] = useState(user?.displayName ?? '');
  const [profileDirty, setProfileDirty] = useState(false);

  // Sync displayName from auth store when it changes (e.g., after save).
  // Uses ref comparison in render to avoid useEffect + setState lint violation.
  if (user?.displayName !== prevDisplayNameRef.current) {
    prevDisplayNameRef.current = user?.displayName;
    setDisplayName(user?.displayName ?? '');
    setProfileDirty(false);
  }

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');

  const handleProfileSave = useCallback(async () => {
    try {
      await updateProfile.mutateAsync({ display_name: displayName });
      setProfileDirty(false);
      useAuthStore.getState().checkSession();
      toast.success('Display name updated');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update profile');
    }
  }, [displayName, updateProfile]);

  const handlePasswordChange = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError('');

    if (newPassword.length < 8) {
      setPasswordError('New password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordError('Passwords do not match');
      return;
    }

    try {
      await changePassword.mutateAsync({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      toast.success('Password changed — please log in again');
      await useAuthStore.getState().logout();
      navigate('/');
      return;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to change password';
      setPasswordError(message);
    }
  }, [currentPassword, newPassword, confirmPassword, changePassword, navigate]);

  return (
    <div className="space-y-8">
      {/* Profile section */}
      <div className="space-y-4">
        <div>
          <h2 className="text-lg font-semibold text-text-primary mb-1">Profile</h2>
          <p className="text-sm text-text-secondary">
            Manage your account information.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-lg">
          <div>
            <label htmlFor="profile-display-name" className="block text-sm font-medium text-text-secondary mb-1.5">
              Display Name
            </label>
            <input
              id="profile-display-name"
              type="text"
              value={displayName}
              onChange={(e) => {
                setDisplayName(e.target.value);
                setProfileDirty(true);
              }}
              placeholder="Your name"
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label htmlFor="profile-email" className="block text-sm font-medium text-text-secondary mb-1.5">
              Email
            </label>
            <input
              id="profile-email"
              type="text"
              value={user?.email ?? ''}
              disabled
              className="w-full px-3 py-2 text-sm bg-bg-tertiary/50 border border-border-primary rounded-md text-text-secondary cursor-not-allowed"
            />
          </div>

          <div>
            <label htmlFor="profile-role" className="block text-sm font-medium text-text-secondary mb-1.5">
              Role
            </label>
            <input
              id="profile-role"
              type="text"
              value={user?.role ?? ''}
              disabled
              className="w-full px-3 py-2 text-sm bg-bg-tertiary/50 border border-border-primary rounded-md text-text-secondary cursor-not-allowed"
            />
          </div>
        </div>

        {profileDirty && (
          <button
            onClick={handleProfileSave}
            disabled={updateProfile.isPending}
            className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50"
          >
            {updateProfile.isPending ? 'Saving...' : 'Save Profile'}
          </button>
        )}
      </div>

      {/* Divider */}
      <div className="border-t border-border-primary" />

      {/* Password section */}
      <div className="space-y-4">
        <div>
          <h2 className="text-lg font-semibold text-text-primary mb-1">Change Password</h2>
          <p className="text-sm text-text-secondary">
            Update your password. Minimum 8 characters.
          </p>
        </div>

        <form onSubmit={handlePasswordChange} className="space-y-4 max-w-sm">
          <div>
            <label htmlFor="current-password" className="block text-sm font-medium text-text-secondary mb-1.5">
              Current Password
            </label>
            <input
              id="current-password"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label htmlFor="new-password" className="block text-sm font-medium text-text-secondary mb-1.5">
              New Password
            </label>
            <input
              id="new-password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              minLength={8}
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label htmlFor="confirm-new-password" className="block text-sm font-medium text-text-secondary mb-1.5">
              Confirm New Password
            </label>
            <input
              id="confirm-new-password"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              minLength={8}
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          {passwordError && (
            <p className="text-sm text-status-error">{passwordError}</p>
          )}

          <button
            type="submit"
            disabled={changePassword.isPending || !currentPassword || !newPassword || !confirmPassword}
            className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50"
          >
            {changePassword.isPending ? 'Changing...' : 'Change Password'}
          </button>
        </form>
      </div>
    </div>
  );
}
