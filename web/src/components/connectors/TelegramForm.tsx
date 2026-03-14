import { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateConnector, useUpdateConnector } from '../../hooks/useBridge.ts';
import type { BridgeConnector } from '../../types/connector.ts';

interface TelegramConfig {
  group_id: number;
  events_topic_id: number;
  commands_topic_id: number;
  poll_timeout: number;
  event_types: string[];
}

interface TelegramFormProps {
  connector?: BridgeConnector;  // undefined = create, defined = edit
  onClose: () => void;
}

function parseTelegramConfig(configJson: string): Partial<TelegramConfig> {
  try {
    return JSON.parse(configJson) as Partial<TelegramConfig>;
  } catch {
    return {};
  }
}

export function TelegramForm({ connector, onClose }: TelegramFormProps) {
  const isEdit = connector !== undefined;
  const existingConfig = isEdit ? parseTelegramConfig(connector.config) : {};

  const [name, setName] = useState(connector?.name ?? '');
  const [botToken, setBotToken] = useState('');
  const [groupId, setGroupId] = useState(String(existingConfig.group_id ?? ''));
  const [eventsTopicId, setEventsTopicId] = useState(String(existingConfig.events_topic_id ?? ''));
  const [commandsTopicId, setCommandsTopicId] = useState(
    String(existingConfig.commands_topic_id ?? ''),
  );
  const [pollTimeout, setPollTimeout] = useState(String(existingConfig.poll_timeout ?? 30));
  const [eventTypes, setEventTypes] = useState(
    (existingConfig.event_types ?? ['session.*', 'run.*']).join(','),
  );

  const createConnector = useCreateConnector();
  const updateConnector = useUpdateConnector();

  const isPending = createConnector.isPending || updateConnector.isPending;

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    const config: TelegramConfig = {
      group_id: Number(groupId),
      events_topic_id: Number(eventsTopicId),
      commands_topic_id: Number(commandsTopicId),
      poll_timeout: Number(pollTimeout),
      event_types: eventTypes
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    };

    const configJson = JSON.stringify(config);
    const configSecret = botToken.trim()
      ? JSON.stringify({ bot_token: botToken.trim() })
      : undefined;

    try {
      if (isEdit) {
        await updateConnector.mutateAsync({
          id: connector.connector_id,
          params: {
            connector_type: 'telegram',
            name: name.trim(),
            config: configJson,
            ...(configSecret !== undefined ? { config_secret: configSecret } : {}),
          },
        });
        toast.success('Connector updated');
      } else {
        await createConnector.mutateAsync({
          connector_type: 'telegram',
          name: name.trim(),
          config: configJson,
          ...(configSecret !== undefined ? { config_secret: configSecret } : {}),
        });
        toast.success('Connector created');
      }
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save connector');
    }
  }

  const inputClass =
    'w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30';

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border-primary rounded-lg shadow-xl max-w-lg w-full mx-4 p-6 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold text-text-primary">
            {isEdit ? 'Edit Telegram Connector' : 'New Telegram Connector'}
          </h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
          >
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {/* Connector name */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Connector name <span className="text-status-error">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. production-telegram"
              required
              autoFocus
              className={inputClass}
            />
          </div>

          {/* Bot token */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Bot token{' '}
              {!isEdit && <span className="text-status-error">*</span>}
              {isEdit && (
                <span className="text-text-secondary/50 ml-1">(leave blank to keep existing)</span>
              )}
            </label>
            <input
              type="password"
              value={botToken}
              onChange={(e) => setBotToken(e.target.value)}
              placeholder="123456:ABC-DEF..."
              required={!isEdit}
              className={inputClass}
            />
          </div>

          {/* Group ID */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Group ID <span className="text-status-error">*</span>
            </label>
            <input
              type="number"
              value={groupId}
              onChange={(e) => setGroupId(e.target.value)}
              placeholder="-1001234567890"
              required
              className={inputClass}
            />
          </div>

          {/* Topic IDs row */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm text-text-secondary mb-1">
                Events topic ID <span className="text-status-error">*</span>
              </label>
              <input
                type="number"
                value={eventsTopicId}
                onChange={(e) => setEventsTopicId(e.target.value)}
                placeholder="1"
                required
                className={inputClass}
              />
            </div>
            <div>
              <label className="block text-sm text-text-secondary mb-1">
                Commands topic ID <span className="text-status-error">*</span>
              </label>
              <input
                type="number"
                value={commandsTopicId}
                onChange={(e) => setCommandsTopicId(e.target.value)}
                placeholder="2"
                required
                className={inputClass}
              />
            </div>
          </div>

          {/* Poll timeout */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Poll timeout (seconds)
            </label>
            <input
              type="number"
              value={pollTimeout}
              onChange={(e) => setPollTimeout(e.target.value)}
              placeholder="30"
              min={1}
              max={300}
              className={inputClass}
            />
          </div>

          {/* Event type filters */}
          <div>
            <label className="block text-sm text-text-secondary mb-1">
              Event type filters{' '}
              <span className="text-text-secondary/50">(comma-separated patterns)</span>
            </label>
            <input
              type="text"
              value={eventTypes}
              onChange={(e) => setEventTypes(e.target.value)}
              placeholder="session.*,run.*"
              className={inputClass}
            />
            <p className="mt-1 text-xs text-text-secondary/50">
              Glob patterns like <code className="font-mono">session.*</code> or{' '}
              <code className="font-mono">run.*</code>
            </p>
          </div>

          <div className="flex justify-end gap-3 mt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isPending}
              className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isPending ? 'Saving...' : isEdit ? 'Save changes' : 'Create connector'}
            </button>
          </div>
        </form>
      </div>
    </div>,
    document.body,
  );
}
