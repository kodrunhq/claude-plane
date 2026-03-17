import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { usersApi } from '../api/users.ts';
import type { ChangePasswordParams, ResetPasswordParams, UpdateProfileParams } from '../api/users.ts';
import type { CreateUserParams, UpdateUserParams } from '../types/user.ts';

export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: () => usersApi.list(),
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateUserParams) => usersApi.create(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: UpdateUserParams }) =>
      usersApi.update(id, params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => usersApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  });
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (params: ChangePasswordParams) => usersApi.changePassword(params),
  });
}

export function useResetPassword() {
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: ResetPasswordParams }) =>
      usersApi.resetPassword(id, params),
  });
}

export function useUpdateProfile() {
  return useMutation({
    mutationFn: (params: UpdateProfileParams) => usersApi.updateProfile(params),
  });
}
