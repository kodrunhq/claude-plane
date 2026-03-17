import { useState, useCallback } from 'react';
import { Save } from 'lucide-react';
import { toast } from 'sonner';
import { EVENT_GROUPS } from '../../constants/eventTypes.ts';
import type { UserPreferences } from '../../types/preferences.ts';

interface NotificationsTabProps {
  preferences: UserPreferences;
  onSave: (prefs: UserPreferences) => Promise<void>;
  saving: boolean;
}

export function NotificationsTab({ preferences, onSave, saving }: NotificationsTabProps) {
  const [selectedEvents, setSelectedEvents] = useState<ReadonlySet<string>>(
    new Set(preferences.notifications?.events ?? []),
  );

  function toggleEvent(eventType: string) {
    setSelectedEvents((prev) => {
      const next = new Set(prev);
      if (next.has(eventType)) {
        next.delete(eventType);
      } else {
        next.add(eventType);
      }
      return next;
    });
  }

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await onSave({
        ...preferences,
        notifications: {
          events: Array.from(selectedEvents),
        },
      });
      toast.success('Notification preferences saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  }, [preferences, selectedEvents, onSave]);

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div>
        <label className="block text-sm font-medium text-text-primary mb-3">
          Subscribe to Event Types
        </label>
        <div className="space-y-4">
          {EVENT_GROUPS.map((group) => (
            <div key={group.label}>
              <h4 className="text-xs font-semibold uppercase tracking-wider text-text-secondary mb-2">
                {group.label}
              </h4>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-1">
                {group.events.map((eventType) => (
                  <label key={eventType} className="flex items-center gap-3 cursor-pointer px-3 py-1.5 rounded-lg hover:bg-bg-tertiary/60 transition-colors">
                    <input
                      type="checkbox"
                      checked={selectedEvents.has(eventType)}
                      onChange={() => toggleEvent(eventType)}
                      className="w-4 h-4 rounded border-border-primary bg-bg-tertiary text-accent-primary focus:ring-accent-primary"
                    />
                    <span className="text-sm text-text-primary font-mono">{eventType}</span>
                  </label>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      <button
        type="submit"
        disabled={saving}
        className="flex items-center gap-2 px-4 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all disabled:opacity-50"
      >
        <Save size={16} />
        {saving ? 'Saving...' : 'Save'}
      </button>
    </form>
  );
}
