import { useState, useCallback } from 'react';
import { X, Send, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import type {
  NotificationChannel,
  SMTPConfig,
  TelegramConfig,
} from '../../types/notification.ts';
import {
  useCreateNotificationChannel,
  useUpdateNotificationChannel,
  useTestNotificationChannel,
} from '../../hooks/useNotifications.ts';

interface ChannelFormModalProps {
  channel?: NotificationChannel;
  onClose: () => void;
}

type TabType = 'email' | 'telegram';

const defaultSMTPConfig: SMTPConfig = {
  host: '',
  port: 587,
  username: '',
  password: '',
  from: '',
  to: '',
  tls: true,
};

const defaultTelegramConfig: TelegramConfig = {
  bot_token: '',
  chat_id: '',
  topic_id: undefined,
};

function parseSMTPConfig(config: string): SMTPConfig {
  try {
    return { ...defaultSMTPConfig, ...JSON.parse(config) } as SMTPConfig;
  } catch {
    return { ...defaultSMTPConfig };
  }
}

function parseTelegramConfig(config: string): TelegramConfig {
  try {
    return { ...defaultTelegramConfig, ...JSON.parse(config) } as TelegramConfig;
  } catch {
    return { ...defaultTelegramConfig };
  }
}

export function ChannelFormModal({ channel, onClose }: ChannelFormModalProps) {
  const isEdit = !!channel;
  const [activeTab, setActiveTab] = useState<TabType>(
    channel?.channel_type ?? 'email',
  );
  const [name, setName] = useState(channel?.name ?? '');
  const [smtpConfig, setSMTPConfig] = useState<SMTPConfig>(() =>
    channel?.channel_type === 'email'
      ? parseSMTPConfig(channel.config)
      : { ...defaultSMTPConfig },
  );
  const [telegramConfig, setTelegramConfig] = useState<TelegramConfig>(() =>
    channel?.channel_type === 'telegram'
      ? parseTelegramConfig(channel.config)
      : { ...defaultTelegramConfig },
  );

  const createMutation = useCreateNotificationChannel();
  const updateMutation = useUpdateNotificationChannel();
  const testMutation = useTestNotificationChannel();

  const saving = createMutation.isPending || updateMutation.isPending;

  const getConfig = useCallback((): string => {
    if (activeTab === 'email') {
      return JSON.stringify(smtpConfig);
    }
    const tg: Record<string, unknown> = {
      bot_token: telegramConfig.bot_token,
      chat_id: telegramConfig.chat_id,
    };
    if (telegramConfig.topic_id && telegramConfig.topic_id > 0) {
      tg.topic_id = telegramConfig.topic_id;
    }
    return JSON.stringify(tg);
  }, [activeTab, smtpConfig, telegramConfig]);

  const handleSave = useCallback(async () => {
    if (!name.trim()) {
      toast.error('Name is required');
      return;
    }

    const config = getConfig();

    try {
      if (isEdit && channel) {
        await updateMutation.mutateAsync({
          id: channel.channel_id,
          params: { name, config, enabled: channel.enabled },
        });
        toast.success('Channel updated');
      } else {
        await createMutation.mutateAsync({
          channel_type: activeTab,
          name,
          config,
        });
        toast.success('Channel created');
      }
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save channel');
    }
  }, [name, getConfig, isEdit, channel, activeTab, createMutation, updateMutation, onClose]);

  const handleTest = useCallback(async () => {
    if (!channel) return;
    try {
      const result = await testMutation.mutateAsync(channel.channel_id);
      if (result.success) {
        toast.success('Test notification sent successfully');
      } else {
        toast.error(`Test failed: ${result.error ?? 'unknown error'}`);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Test failed');
    }
  }, [channel, testMutation]);

  const inputClass =
    'w-full px-3 py-2 text-sm rounded-lg border border-border-primary bg-bg-secondary text-text-primary focus:outline-none focus:ring-1 focus:ring-accent-primary';
  const labelClass = 'block text-sm font-medium text-text-secondary mb-1';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-bg-primary border border-border-primary rounded-xl shadow-xl w-full max-w-lg mx-4">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border-primary">
          <h3 className="text-lg font-semibold text-text-primary">
            {isEdit ? 'Edit Channel' : 'New Notification Channel'}
          </h3>
          <button
            onClick={onClose}
            className="p-1 rounded-lg hover:bg-bg-tertiary transition-colors text-text-secondary"
            aria-label="Close"
          >
            <X size={20} />
          </button>
        </div>

        <div className="px-6 py-4 space-y-4 max-h-[70vh] overflow-y-auto">
          {/* Name */}
          <div>
            <label className={labelClass}>Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Team Alerts"
              className={inputClass}
            />
          </div>

          {/* Channel type tabs */}
          {!isEdit && (
            <div className="flex gap-2">
              {(['email', 'telegram'] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setActiveTab(tab)}
                  className={`px-4 py-2 text-sm rounded-lg font-medium transition-colors ${
                    activeTab === tab
                      ? 'bg-accent-primary text-white'
                      : 'bg-bg-tertiary text-text-secondary hover:text-text-primary'
                  }`}
                >
                  {tab === 'email' ? 'Email (SMTP)' : 'Telegram'}
                </button>
              ))}
            </div>
          )}

          {/* Email config */}
          {activeTab === 'email' && (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelClass}>SMTP Host</label>
                  <input
                    type="text"
                    value={smtpConfig.host}
                    onChange={(e) =>
                      setSMTPConfig({ ...smtpConfig, host: e.target.value })
                    }
                    placeholder="smtp.gmail.com"
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className={labelClass}>Port</label>
                  <input
                    type="number"
                    value={smtpConfig.port}
                    onChange={(e) =>
                      setSMTPConfig({
                        ...smtpConfig,
                        port: parseInt(e.target.value, 10) || 587,
                      })
                    }
                    className={inputClass}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelClass}>Username</label>
                  <input
                    type="text"
                    value={smtpConfig.username}
                    onChange={(e) =>
                      setSMTPConfig({ ...smtpConfig, username: e.target.value })
                    }
                    placeholder="user@gmail.com"
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className={labelClass}>Password</label>
                  <input
                    type="password"
                    value={smtpConfig.password}
                    onChange={(e) =>
                      setSMTPConfig({ ...smtpConfig, password: e.target.value })
                    }
                    placeholder="app password"
                    className={inputClass}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className={labelClass}>From</label>
                  <input
                    type="email"
                    value={smtpConfig.from}
                    onChange={(e) =>
                      setSMTPConfig({ ...smtpConfig, from: e.target.value })
                    }
                    placeholder="noreply@example.com"
                    className={inputClass}
                  />
                </div>
                <div>
                  <label className={labelClass}>To</label>
                  <input
                    type="email"
                    value={smtpConfig.to}
                    onChange={(e) =>
                      setSMTPConfig({ ...smtpConfig, to: e.target.value })
                    }
                    placeholder="team@example.com"
                    className={inputClass}
                  />
                </div>

                <div className="flex items-center justify-between py-2">
                  <div>
                    <span className="text-sm font-medium text-text-primary">TLS / STARTTLS</span>
                    <p className="text-xs text-text-secondary">Required for Gmail, SendGrid, and most providers</p>
                  </div>
                  <button
                    type="button"
                    onClick={() => setSMTPConfig({ ...smtpConfig, tls: !smtpConfig.tls })}
                    className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none ${
                      smtpConfig.tls ? 'bg-accent-primary' : 'bg-gray-600'
                    }`}
                    role="switch"
                    aria-checked={smtpConfig.tls}
                  >
                    <span
                      className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                        smtpConfig.tls ? 'translate-x-[18px]' : 'translate-x-0.5'
                      }`}
                    />
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Telegram config */}
          {activeTab === 'telegram' && (
            <div className="space-y-3">
              <div>
                <label className={labelClass}>Bot Token</label>
                <input
                  type="password"
                  value={telegramConfig.bot_token}
                  onChange={(e) =>
                    setTelegramConfig({
                      ...telegramConfig,
                      bot_token: e.target.value,
                    })
                  }
                  placeholder="123456:ABC-DEF..."
                  className={inputClass}
                />
              </div>
              <div>
                <label className={labelClass}>Chat ID</label>
                <input
                  type="text"
                  value={telegramConfig.chat_id}
                  onChange={(e) =>
                    setTelegramConfig({
                      ...telegramConfig,
                      chat_id: e.target.value,
                    })
                  }
                  placeholder="-1001234567890"
                  className={inputClass}
                />
              </div>
              <div>
                <label className={labelClass}>Topic ID (optional)</label>
                <input
                  type="number"
                  value={telegramConfig.topic_id ?? ''}
                  onChange={(e) =>
                    setTelegramConfig({
                      ...telegramConfig,
                      topic_id: e.target.value
                        ? parseInt(e.target.value, 10)
                        : undefined,
                    })
                  }
                  placeholder="For forum-style groups"
                  className={inputClass}
                />
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-border-primary">
          <div>
            {isEdit && channel && (
              <button
                onClick={handleTest}
                disabled={testMutation.isPending}
                className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg font-medium text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors disabled:opacity-50"
              >
                {testMutation.isPending ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <Send size={14} />
                )}
                Test
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-4 py-2 text-sm rounded-lg font-medium text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="px-4 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all disabled:opacity-50"
            >
              {saving ? 'Saving...' : isEdit ? 'Update' : 'Create'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
