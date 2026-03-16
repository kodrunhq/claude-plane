import { useState, useCallback } from 'react';
import { Save } from 'lucide-react';
import { toast } from 'sonner';
import type { UserPreferences, UIPrefs } from '../../types/preferences.ts';

interface UIPreferencesTabProps {
  preferences: UserPreferences;
  onSave: (prefs: UserPreferences) => Promise<void>;
  saving: boolean;
}

const CARD_OPTIONS = [
  { value: 'sessions', label: 'Sessions' },
  { value: 'machines', label: 'Machines' },
  { value: 'jobs', label: 'Jobs' },
  { value: 'runs', label: 'Runs' },
  { value: 'templates', label: 'Templates' },
] as const;

const THEME_OPTIONS: ReadonlyArray<{ value: UIPrefs['theme']; label: string }> = [
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
  { value: 'system', label: 'System' },
];

export function UIPreferencesTab({ preferences, onSave, saving }: UIPreferencesTabProps) {
  const [theme, setTheme] = useState<UIPrefs['theme']>(preferences.ui?.theme ?? 'system');
  const [fontSize, setFontSize] = useState(String(preferences.ui?.terminal_font_size ?? 14));
  const [autoAttach, setAutoAttach] = useState(preferences.ui?.auto_attach_session ?? false);
  const [selectedCards, setSelectedCards] = useState<ReadonlySet<string>>(
    new Set(preferences.ui?.command_center_cards ?? ['sessions', 'machines', 'jobs', 'runs']),
  );

  function toggleCard(card: string) {
    setSelectedCards((prev) => {
      const next = new Set(prev);
      if (next.has(card)) {
        next.delete(card);
      } else {
        next.add(card);
      }
      return next;
    });
  }

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await onSave({
        ...preferences,
        ui: {
          theme,
          terminal_font_size: Number(fontSize) || 14,
          auto_attach_session: autoAttach,
          command_center_cards: Array.from(selectedCards),
        },
      });
      toast.success('UI preferences saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  }, [preferences, theme, fontSize, autoAttach, selectedCards, onSave]);

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">Theme</label>
        <select
          value={theme}
          onChange={(e) => setTheme(e.target.value as UIPrefs['theme'])}
          className="w-full sm:w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:ring-1 focus:ring-accent-primary"
        >
          {THEME_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-sm font-medium text-text-primary mb-1.5">
          Terminal Font Size
        </label>
        <input
          type="number"
          value={fontSize}
          onChange={(e) => setFontSize(e.target.value)}
          min={8}
          max={32}
          className="w-full sm:w-48 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:ring-1 focus:ring-accent-primary"
        />
        <p className="text-xs text-text-secondary mt-1">8-32px</p>
      </div>

      <div>
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            checked={autoAttach}
            onChange={(e) => setAutoAttach(e.target.checked)}
            className="w-4 h-4 rounded border-border-primary bg-bg-tertiary text-accent-primary focus:ring-accent-primary"
          />
          <div>
            <p className="text-sm font-medium text-text-primary">Auto-Attach Sessions</p>
            <p className="text-xs text-text-secondary">Automatically attach to new sessions when created</p>
          </div>
        </label>
      </div>

      <div>
        <label className="block text-sm font-medium text-text-primary mb-3">
          Command Center Cards
        </label>
        <div className="flex flex-wrap gap-2">
          {CARD_OPTIONS.map((opt) => (
            <label key={opt.value} className="flex items-center gap-2 cursor-pointer px-3 py-2 rounded-lg hover:bg-bg-tertiary/60 transition-colors">
              <input
                type="checkbox"
                checked={selectedCards.has(opt.value)}
                onChange={() => toggleCard(opt.value)}
                className="w-4 h-4 rounded border-border-primary bg-bg-tertiary text-accent-primary focus:ring-accent-primary"
              />
              <span className="text-sm text-text-primary">{opt.label}</span>
            </label>
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
