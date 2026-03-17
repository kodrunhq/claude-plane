import { request } from './client.ts';
import type {
  NotificationChannel,
  NotificationSubscription,
  CreateChannelParams,
  UpdateChannelParams,
  TestChannelResult,
} from '../types/notification.ts';

export const notificationsApi = {
  listChannels: () =>
    request<NotificationChannel[]>('/notification-channels'),

  getChannel: (id: string) =>
    request<NotificationChannel>(`/notification-channels/${encodeURIComponent(id)}`),

  createChannel: (params: CreateChannelParams) =>
    request<NotificationChannel>('/notification-channels', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  updateChannel: (id: string, params: UpdateChannelParams) =>
    request<NotificationChannel>(`/notification-channels/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),

  deleteChannel: (id: string) =>
    request<void>(`/notification-channels/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  testChannel: (id: string) =>
    request<TestChannelResult>(`/notification-channels/${encodeURIComponent(id)}/test`, {
      method: 'POST',
    }),

  getSubscriptions: () =>
    request<NotificationSubscription[]>('/notifications/subscriptions'),

  setSubscriptions: (subscriptions: NotificationSubscription[]) =>
    request<void>('/notifications/subscriptions', {
      method: 'PUT',
      body: JSON.stringify({ subscriptions }),
    }),
};
