import { useState } from 'react';
import { Plus, Users, AlertCircle, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { useUsers, useCreateUser, useUpdateUser } from '../hooks/useUsers.ts';
import { UsersList } from '../components/admin/UsersList.tsx';
import { CreateUserModal } from '../components/admin/CreateUserModal.tsx';
import { EditUserModal } from '../components/admin/EditUserModal.tsx';
import { SkeletonTable } from '../components/shared/SkeletonTable.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import type { User, CreateUserParams, UpdateUserParams } from '../types/user.ts';

export function AdminPage() {
  const { data: users, isLoading, error, refetch } = useUsers();
  const createUser = useCreateUser();
  const updateUser = useUpdateUser();

  const [showCreate, setShowCreate] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);

  async function handleCreate(params: CreateUserParams) {
    try {
      await createUser.mutateAsync(params);
      toast.success('User created');
      setShowCreate(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create user');
    }
  }

  async function handleUpdate(id: string, params: UpdateUserParams) {
    try {
      await updateUser.mutateAsync({ id, params });
      toast.success('User updated');
      setEditingUser(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update user');
    }
  }

  if (error) {
    return (
      <div className="p-6">
        <div className="bg-status-error/10 border border-status-error/30 rounded-lg p-4 flex items-center gap-3">
          <AlertCircle className="text-status-error shrink-0" size={20} />
          <p className="text-sm text-text-primary flex-1">
            {error instanceof Error ? error.message : 'Failed to load users'}
          </p>
          <button
            onClick={() => refetch()}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-text-primary">Users</h1>
          <p className="text-sm text-text-secondary mt-0.5">Manage user accounts and roles</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Plus size={16} />
          New User
        </button>
      </div>

      {isLoading ? (
        <SkeletonTable rows={4} columns={5} />
      ) : !users || users.length === 0 ? (
        <EmptyState
          icon={<Users size={40} />}
          title="No users yet"
          description="Create user accounts to grant access to claude-plane."
          action={
            <button
              onClick={() => setShowCreate(true)}
              className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
            >
              <Plus size={16} />
              New User
            </button>
          }
        />
      ) : (
        <UsersList users={users} onEdit={setEditingUser} />
      )}

      <CreateUserModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onSubmit={handleCreate}
        submitting={createUser.isPending}
      />

      <EditUserModal
        open={!!editingUser}
        user={editingUser}
        onClose={() => setEditingUser(null)}
        onSubmit={handleUpdate}
        submitting={updateUser.isPending}
      />
    </div>
  );
}
