import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { notificationsApi } from '../api/notifications.ts';
import type {
  CreateChannelParams,
  UpdateChannelParams,
  NotificationSubscription,
} from '../types/notification.ts';

export function useNotificationChannels() {
  return useQuery({
    queryKey: ['notification-channels'],
    queryFn: () => notificationsApi.listChannels(),
  });
}

export function useNotificationChannel(id: string | undefined) {
  return useQuery({
    queryKey: ['notification-channels', id],
    queryFn: () => notificationsApi.getChannel(id!),
    enabled: !!id,
  });
}

export function useCreateNotificationChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateChannelParams) => notificationsApi.createChannel(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notification-channels'] }),
  });
}

export function useUpdateNotificationChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: UpdateChannelParams }) =>
      notificationsApi.updateChannel(id, params),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['notification-channels', id] });
      qc.invalidateQueries({ queryKey: ['notification-channels'] });
    },
  });
}

export function useDeleteNotificationChannel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => notificationsApi.deleteChannel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notification-channels'] }),
  });
}

export function useTestNotificationChannel() {
  return useMutation({
    mutationFn: (id: string) => notificationsApi.testChannel(id),
  });
}

export function useNotificationSubscriptions() {
  return useQuery({
    queryKey: ['notification-subscriptions'],
    queryFn: () => notificationsApi.getSubscriptions(),
  });
}

export function useSetNotificationSubscriptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (subscriptions: NotificationSubscription[]) =>
      notificationsApi.setSubscriptions(subscriptions),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notification-subscriptions'] }),
  });
}
