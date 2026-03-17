import { useState, useMemo } from 'react';
import { EVENT_GROUPS } from '../../constants/eventTypes.ts';
import type { Webhook, CreateWebhookParams, UpdateWebhookParams } from '../../types/webhook.ts';

interface WebhookFormProps {
  initial?: Webhook;
  onSubmit: (params: CreateWebhookParams | UpdateWebhookParams) => void;
  onCancel: () => void;
  submitting?: boolean;
}

export function WebhookForm({ initial, onSubmit, onCancel, submitting = false }: WebhookFormProps) {
  const allEvents = useMemo(() => EVENT_GROUPS.flatMap((g) => g.events), []);
  const [name, setName] = useState(initial?.name ?? '');
  const [url, setUrl] = useState(initial?.url ?? '');
  const [secret, setSecret] = useState('');
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(
    new Set(initial?.events ?? []),
  );
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [errors, setErrors] = useState<Record<string, string>>({});

  function validate(): boolean {
    const next: Record<string, string> = {};
    if (!name.trim()) next.name = 'Name is required';
    if (!url.trim()) next.url = 'URL is required';
    else {
      try {
        new URL(url);
      } catch {
        next.url = 'Must be a valid URL';
      }
    }
    if (selectedEvents.size === 0) next.events = 'Select at least one event';
    setErrors(next);
    return Object.keys(next).length === 0;
  }

  function handleToggleEvent(event: string) {
    const next = new Set(selectedEvents);
    if (next.has(event)) {
      next.delete(event);
    } else {
      next.add(event);
    }
    setSelectedEvents(next);
  }

  function handleToggleGroup(events: string[]) {
    const allSelected = events.every((e) => selectedEvents.has(e));
    const next = new Set(selectedEvents);
    if (allSelected) {
      events.forEach((e) => next.delete(e));
    } else {
      events.forEach((e) => next.add(e));
    }
    setSelectedEvents(next);
  }

  function handleSelectAll() {
    if (selectedEvents.size === allEvents.length) {
      setSelectedEvents(new Set());
    } else {
      setSelectedEvents(new Set(allEvents));
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;

    const params: CreateWebhookParams = {
      name: name.trim(),
      url: url.trim(),
      events: Array.from(selectedEvents),
      enabled,
    };
    if (secret) {
      params.secret = secret;
    }
    onSubmit(params);
  }

  const isEditing = !!initial;
  const allSelected = selectedEvents.size === allEvents.length;

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <div className="space-y-1">
        <label className="block text-sm font-medium text-text-primary">Name</label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="My webhook"
          className={`w-full px-3 py-2 bg-bg-tertiary border ${errors.name ? 'border-status-error' : 'border-border-primary'} rounded-md text-sm text-text-primary placeholder:text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent-primary`}
        />
        {errors.name && <p className="text-xs text-status-error">{errors.name}</p>}
      </div>

      <div className="space-y-1">
        <label className="block text-sm font-medium text-text-primary">Endpoint URL</label>
        <input
          type="url"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://example.com/webhook"
          className={`w-full px-3 py-2 bg-bg-tertiary border ${errors.url ? 'border-status-error' : 'border-border-primary'} rounded-md text-sm text-text-primary placeholder:text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent-primary`}
        />
        {errors.url && <p className="text-xs text-status-error">{errors.url}</p>}
      </div>

      <div className="space-y-1">
        <label className="block text-sm font-medium text-text-primary">
          Secret{' '}
          <span className="text-text-secondary font-normal">
            {isEditing ? '(leave blank to keep existing)' : '(optional)'}
          </span>
        </label>
        <input
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          placeholder={isEditing ? '••••••••' : 'HMAC signing secret'}
          className="w-full px-3 py-2 bg-bg-tertiary border border-border-primary rounded-md text-sm text-text-primary placeholder:text-text-secondary focus:outline-none focus:ring-1 focus:ring-accent-primary"
        />
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="block text-sm font-medium text-text-primary">Events</label>
          <button
            type="button"
            onClick={handleSelectAll}
            className="text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
          >
            {allSelected ? 'Deselect all' : 'Select all'}
          </button>
        </div>

        <div className={`border ${errors.events ? 'border-status-error' : 'border-border-primary'} rounded-md divide-y divide-gray-700`}>
          {EVENT_GROUPS.map((group) => {
            const groupAllSelected = group.events.every((e) => selectedEvents.has(e));
            return (
              <div key={group.label} className="p-3 space-y-2">
                <label className="flex items-center gap-2 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={groupAllSelected}
                    onChange={() => handleToggleGroup(group.events)}
                    className="accent-accent-primary"
                  />
                  <span className="text-xs font-semibold uppercase tracking-wider text-text-secondary">
                    {group.label}
                  </span>
                </label>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-1 pl-5">
                  {group.events.map((event) => (
                    <label
                      key={event}
                      className="flex items-center gap-2 cursor-pointer select-none"
                    >
                      <input
                        type="checkbox"
                        checked={selectedEvents.has(event)}
                        onChange={() => handleToggleEvent(event)}
                        className="accent-accent-primary"
                      />
                      <span className="text-xs text-text-secondary font-mono">{event}</span>
                    </label>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
        {errors.events && <p className="text-xs text-status-error">{errors.events}</p>}
      </div>

      <div className="flex items-center justify-between py-2">
        <span className="text-sm font-medium text-text-primary">Enabled</span>
        <button
          type="button"
          onClick={() => setEnabled((v) => !v)}
          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors focus:outline-none ${
            enabled ? 'bg-accent-primary' : 'bg-gray-600'
          }`}
          role="switch"
          aria-checked={enabled}
        >
          <span
            className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
              enabled ? 'translate-x-[18px]' : 'translate-x-0.5'
            }`}
          />
        </button>
      </div>

      <div className="flex justify-end gap-3 pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting}
          className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {submitting ? 'Saving...' : isEditing ? 'Update webhook' : 'Create webhook'}
        </button>
      </div>
    </form>
  );
}
