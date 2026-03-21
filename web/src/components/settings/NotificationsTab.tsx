import { useState, useCallback, useMemo, Fragment } from 'react';
import { Plus, Pencil, Trash2, Send, Loader2, Mail, MessageCircle, Link as LinkIcon, Info } from 'lucide-react';
import { Link } from 'react-router';
import { toast } from 'sonner';
import { EVENT_GROUPS } from '../../constants/eventTypes.ts';
import type { NotificationChannel, NotificationSubscription } from '../../types/notification.ts';
import {
  useNotificationChannels,
  useDeleteNotificationChannel,
  useTestNotificationChannel,
  useNotificationSubscriptions,
  useSetNotificationSubscriptions,
} from '../../hooks/useNotifications.ts';
import { ChannelFormModal } from './ChannelFormModal.tsx';

export function NotificationsTab() {
  const [modalOpen, setModalOpen] = useState(false);
  const [editingChannel, setEditingChannel] = useState<NotificationChannel | undefined>();

  const { data: channels = [], isLoading: channelsLoading } = useNotificationChannels();
  const { data: subscriptions = [], isLoading: subsLoading } = useNotificationSubscriptions();
  const deleteMutation = useDeleteNotificationChannel();
  const testMutation = useTestNotificationChannel();
  const setSubs = useSetNotificationSubscriptions();

  // Build a Set for quick lookup: "channelId:eventType"
  const subSet = useMemo(() => {
    const s = new Set<string>();
    for (const sub of subscriptions) {
      s.add(`${sub.channel_id}:${sub.event_type}`);
    }
    return s;
  }, [subscriptions]);

  const handleOpenCreate = useCallback(() => {
    setEditingChannel(undefined);
    setModalOpen(true);
  }, []);

  const handleOpenEdit = useCallback((ch: NotificationChannel) => {
    setEditingChannel(ch);
    setModalOpen(true);
  }, []);

  const handleCloseModal = useCallback(() => {
    setModalOpen(false);
    setEditingChannel(undefined);
  }, []);

  const handleDelete = useCallback(
    async (ch: NotificationChannel) => {
      if (!confirm(`Delete channel "${ch.name}"? This will remove all subscriptions for it.`)) {
        return;
      }
      try {
        await deleteMutation.mutateAsync(ch.channel_id);
        toast.success('Channel deleted');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to delete');
      }
    },
    [deleteMutation],
  );

  const handleTest = useCallback(
    async (ch: NotificationChannel) => {
      try {
        const result = await testMutation.mutateAsync(ch.channel_id);
        if (result.success) {
          toast.success('Test notification sent');
        } else {
          toast.error(`Test failed: ${result.error ?? 'unknown error'}`);
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Test failed');
      }
    },
    [testMutation],
  );

  const toggleSubscription = useCallback(
    (channelId: string, eventType: string) => {
      const key = `${channelId}:${eventType}`;
      const newSubs: NotificationSubscription[] = [];

      const found = subSet.has(key);
      for (const sub of subscriptions) {
        const subKey = `${sub.channel_id}:${sub.event_type}`;
        if (subKey === key) continue;
        newSubs.push(sub);
      }
      if (!found) {
        newSubs.push({ channel_id: channelId, event_type: eventType });
      }

      setSubs.mutate(newSubs);
    },
    [subscriptions, subSet, setSubs],
  );

  const toggleAllForEvent = useCallback(
    (eventType: string) => {
      const allChecked = channels.every((ch) => subSet.has(`${ch.channel_id}:${eventType}`));

      const newSubs: NotificationSubscription[] = [];
      for (const sub of subscriptions) {
        if (sub.event_type !== eventType) {
          newSubs.push(sub);
        }
      }
      if (!allChecked) {
        for (const ch of channels) {
          newSubs.push({ channel_id: ch.channel_id, event_type: eventType });
        }
      }

      setSubs.mutate(newSubs);
    },
    [channels, subscriptions, subSet, setSubs],
  );

  const toggleAllForChannel = useCallback(
    (channelId: string) => {
      const allEvents = EVENT_GROUPS.flatMap((g) => g.events);
      const allChecked = allEvents.every((evt) => subSet.has(`${channelId}:${evt}`));

      const newSubs: NotificationSubscription[] = [];
      for (const sub of subscriptions) {
        if (sub.channel_id !== channelId) {
          newSubs.push(sub);
        }
      }
      if (!allChecked) {
        for (const evt of allEvents) {
          newSubs.push({ channel_id: channelId, event_type: evt });
        }
      }

      setSubs.mutate(newSubs);
    },
    [subscriptions, subSet, setSubs],
  );

  if (channelsLoading || subsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 size={24} className="animate-spin text-text-secondary" />
      </div>
    );
  }

  const channelIcon = (type: string) =>
    type === 'email' ? <Mail size={16} /> : <MessageCircle size={16} />;

  return (
    <div className="space-y-8">
      {/* Section 1: Channels */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-base font-semibold text-text-primary">Channels</h3>
            <p className="text-sm text-text-secondary">
              Configure where notifications are delivered.
            </p>
          </div>
          <button
            onClick={handleOpenCreate}
            className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all"
          >
            <Plus size={16} />
            Add Channel
          </button>
        </div>

        {channels.length === 0 ? (
          <div className="text-center py-8 text-text-secondary text-sm border border-border-primary rounded-lg bg-bg-secondary">
            No notification channels configured yet.
          </div>
        ) : (
          <div className="space-y-2">
            {channels.map((ch) => {
              const isConnectorBacked = !!ch.connector_id;
              return (
                <div
                  key={ch.channel_id}
                  className="flex items-center justify-between px-4 py-3 rounded-lg border border-border-primary bg-bg-secondary"
                >
                  <div className="flex items-center gap-3">
                    <span className="text-text-secondary">{channelIcon(ch.channel_type)}</span>
                    <div>
                      <span className="text-sm font-medium text-text-primary">{ch.name}</span>
                      <span className="ml-2 text-xs text-text-tertiary">
                        {ch.channel_type}
                      </span>
                    </div>
                    {!ch.enabled && (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-bg-tertiary text-text-secondary">
                        disabled
                      </span>
                    )}
                    {isConnectorBacked && (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-accent-primary/15 text-accent-primary font-medium">
                        Connector
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => handleTest(ch)}
                      disabled={testMutation.isPending}
                      className="p-1.5 rounded-lg hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
                      title="Send test notification"
                    >
                      <Send size={14} />
                    </button>
                    {isConnectorBacked ? (
                      <Link
                        to={`/connectors/${ch.connector_id}`}
                        className="p-1.5 rounded-lg hover:bg-bg-tertiary text-text-secondary hover:text-accent-primary transition-colors"
                        title="Managed from Connectors page"
                      >
                        <LinkIcon size={14} />
                      </Link>
                    ) : (
                      <>
                        <button
                          onClick={() => handleOpenEdit(ch)}
                          className="p-1.5 rounded-lg hover:bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
                          title="Edit channel"
                        >
                          <Pencil size={14} />
                        </button>
                        <button
                          onClick={() => handleDelete(ch)}
                          disabled={deleteMutation.isPending}
                          className="p-1.5 rounded-lg hover:bg-bg-tertiary text-text-secondary hover:text-status-error transition-colors"
                          title="Delete channel"
                        >
                          <Trash2 size={14} />
                        </button>
                      </>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Telegram connector info banner */}
      {!channels.some((ch) => ch.connector_id && ch.channel_type === 'telegram') && (
        <div className="flex items-start gap-2 rounded-md bg-blue-500/10 border border-blue-500/20 px-3 py-2">
          <Info size={14} className="text-blue-400 mt-0.5 shrink-0" />
          <p className="text-xs text-blue-300">
            Want Telegram notifications?{' '}
            <Link to="/connectors" className="underline hover:text-blue-200">
              Set up a Telegram connector
            </Link>{' '}
            first.
          </p>
        </div>
      )}

      {/* Section 2: Subscription Matrix */}
      {channels.length > 0 && (
        <div>
          <div className="mb-4">
            <h3 className="text-base font-semibold text-text-primary">Subscriptions</h3>
            <p className="text-sm text-text-secondary">
              Choose which events are sent to each channel.
            </p>
          </div>

          <div className="overflow-x-auto border border-border-primary rounded-lg">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-bg-secondary">
                  <th className="text-left px-4 py-3 font-medium text-text-secondary">Event</th>
                  {channels.map((ch) => (
                    <th
                      key={ch.channel_id}
                      className="px-3 py-3 font-medium text-text-secondary text-center whitespace-nowrap"
                    >
                      <div className="flex flex-col items-center gap-1">
                        <span className="flex items-center gap-1">
                          {channelIcon(ch.channel_type)}
                          {ch.name}
                        </span>
                        <button
                          onClick={() => toggleAllForChannel(ch.channel_id)}
                          className="text-xs text-accent-primary hover:underline"
                        >
                          toggle all
                        </button>
                      </div>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {EVENT_GROUPS.map((group) => (
                  <Fragment key={`group-${group.label}`}>
                    <tr>
                      <td
                        colSpan={channels.length + 1}
                        className="px-4 py-2 text-xs font-semibold uppercase tracking-wider text-text-secondary bg-bg-tertiary/50"
                      >
                        {group.label}
                      </td>
                    </tr>
                    {group.events.map((eventType) => (
                      <tr
                        key={eventType}
                        className="border-t border-border-primary hover:bg-bg-tertiary/30 transition-colors group"
                      >
                        <td className="px-4 py-2">
                          <div className="flex items-center gap-2">
                            <span className="font-mono text-text-primary">{eventType}</span>
                            <button
                              onClick={() => toggleAllForEvent(eventType)}
                              className="text-xs text-accent-primary hover:underline opacity-0 group-hover:opacity-100 transition-opacity"
                              title="Toggle all channels for this event"
                            >
                              all
                            </button>
                          </div>
                        </td>
                        {channels.map((ch) => (
                          <td key={ch.channel_id} className="px-3 py-2 text-center">
                            <input
                              type="checkbox"
                              checked={subSet.has(`${ch.channel_id}:${eventType}`)}
                              onChange={() => toggleSubscription(ch.channel_id, eventType)}
                              className="w-4 h-4 rounded border-border-primary bg-bg-tertiary text-accent-primary focus:ring-accent-primary cursor-pointer"
                            />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>

          {setSubs.isPending && (
            <div className="flex items-center gap-2 mt-2 text-sm text-text-secondary">
              <Loader2 size={14} className="animate-spin" />
              Saving subscriptions...
            </div>
          )}
        </div>
      )}

      {/* Modal */}
      {modalOpen && (
        <ChannelFormModal channel={editingChannel} onClose={handleCloseModal} />
      )}
    </div>
  );
}
