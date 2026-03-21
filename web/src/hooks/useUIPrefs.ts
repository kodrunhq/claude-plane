import { usePreferences } from './usePreferences.ts';
import type { UIPrefs } from '../types/preferences.ts';

const DEFAULTS: UIPrefs = {
  theme: 'system',
  terminal_font_size: 14,
  command_center_cards: ['sessions', 'machines', 'jobs', 'runs', 'templates', 'health'],
};

/** Returns resolved UI preferences with defaults applied. */
export function useUIPrefs(): UIPrefs {
  const { data } = usePreferences();
  const ui = data?.ui;
  return {
    theme: ui?.theme ?? DEFAULTS.theme,
    terminal_font_size: ui?.terminal_font_size ?? DEFAULTS.terminal_font_size,
    command_center_cards: ui?.command_center_cards ?? DEFAULTS.command_center_cards,
  };
}
