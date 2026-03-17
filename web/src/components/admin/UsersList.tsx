import { useState } from 'react';
import { Pencil, Trash2, ShieldCheck, User, KeyRound } from 'lucide-react';
import { toast } from 'sonner';
import { ConfirmDialog } from '../shared/ConfirmDialog.tsx';
import { formatTimeAgo, truncateId } from '../../lib/format.ts';
import { useDeleteUser } from '../../hooks/useUsers.ts';
import type { User as UserType } from '../../types/user.ts';

interface UsersListProps {
  users: UserType[];
  onEdit: (user: UserType) => void;
  onResetPassword: (user: UserType) => void;
}

interface UserRowProps {
  user: UserType;
  onEdit: (user: UserType) => void;
  onDeleteRequest: (user: UserType) => void;
  onResetPassword: (user: UserType) => void;
}

function RoleBadge({ role }: { role: string }) {
  const isAdmin = role === 'admin';
  return (
    <span
      className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full font-medium ${
        isAdmin
          ? 'bg-accent-primary/20 text-accent-primary'
          : 'bg-bg-tertiary text-text-secondary'
      }`}
    >
      {isAdmin ? <ShieldCheck size={11} /> : <User size={11} />}
      {role}
    </span>
  );
}

function UserRow({ user, onEdit, onDeleteRequest, onResetPassword }: UserRowProps) {
  return (
    <tr className="border-t border-gray-800 hover:bg-bg-tertiary/20 transition-colors">
      <td className="px-4 py-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium text-text-primary">
            {user.display_name || <span className="text-text-secondary italic">No name</span>}
          </span>
          <span className="text-xs text-text-secondary font-mono">{truncateId(user.user_id)}</span>
        </div>
      </td>
      <td className="px-4 py-3 text-sm text-text-secondary font-mono truncate max-w-[240px]">
        {user.email}
      </td>
      <td className="px-4 py-3">
        <RoleBadge role={user.role} />
      </td>
      <td className="px-4 py-3 text-sm text-text-secondary">
        {formatTimeAgo(user.created_at)}
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1 justify-end">
          <button
            onClick={() => onResetPassword(user)}
            className="p-2.5 md:p-1.5 rounded text-text-secondary hover:text-accent-primary hover:bg-bg-tertiary transition-colors"
            title="Reset password"
          >
            <KeyRound size={15} />
          </button>
          <button
            onClick={() => onEdit(user)}
            className="p-2.5 md:p-1.5 rounded text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
            title="Edit user"
          >
            <Pencil size={15} />
          </button>
          <button
            onClick={() => onDeleteRequest(user)}
            className="p-2.5 md:p-1.5 rounded text-text-secondary hover:text-status-error hover:bg-bg-tertiary transition-colors"
            title="Delete user"
          >
            <Trash2 size={15} />
          </button>
        </div>
      </td>
    </tr>
  );
}

export function UsersList({ users, onEdit, onResetPassword }: UsersListProps) {
  const [pendingDelete, setPendingDelete] = useState<UserType | null>(null);
  const deleteUser = useDeleteUser();

  async function handleConfirmDelete() {
    if (!pendingDelete) return;
    try {
      await deleteUser.mutateAsync(pendingDelete.user_id);
      toast.success('User deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete user');
    } finally {
      setPendingDelete(null);
    }
  }

  return (
    <>
      <div className="overflow-x-auto rounded-lg border border-border-primary">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="bg-bg-secondary">
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Name
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Email
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Role
              </th>
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Created
              </th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {users.map((user) => (
              <UserRow
                key={user.user_id}
                user={user}
                onEdit={onEdit}
                onDeleteRequest={setPendingDelete}
                onResetPassword={onResetPassword}
              />
            ))}
          </tbody>
        </table>
      </div>

      <ConfirmDialog
        open={!!pendingDelete}
        title="Delete user"
        message={`Are you sure you want to delete "${pendingDelete?.email}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleConfirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </>
  );
}
