import { useState, useCallback } from 'react';
import { toast } from 'sonner';
import { useAuthStore } from '../../stores/auth.ts';
import { useChangePassword, useUpdateProfile } from '../../hooks/useUsers.ts';

export function AccountTab() {
  const user = useAuthStore((s) => s.user);
  const changePassword = useChangePassword();
  const updateProfile = useUpdateProfile();

  const [displayName, setDisplayName] = useState(user?.displayName ?? '');
  const [profileDirty, setProfileDirty] = useState(false);

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');

  const handleProfileSave = useCallback(async () => {
    try {
      await updateProfile.mutateAsync({ display_name: displayName });
      setProfileDirty(false);
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
      toast.success('Password changed successfully');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to change password';
      setPasswordError(message);
    }
  }, [currentPassword, newPassword, confirmPassword, changePassword]);

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
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Display Name
            </label>
            <input
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
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Email
            </label>
            <input
              type="text"
              value={user?.email ?? ''}
              disabled
              className="w-full px-3 py-2 text-sm bg-bg-tertiary/50 border border-border-primary rounded-md text-text-secondary cursor-not-allowed"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Role
            </label>
            <input
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
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Current Password
            </label>
            <input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              New Password
            </label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              minLength={8}
              className="w-full px-3 py-2 text-sm bg-bg-tertiary border border-border-primary rounded-md text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1.5">
              Confirm New Password
            </label>
            <input
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
