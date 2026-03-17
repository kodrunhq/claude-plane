import { useEffect, useRef } from 'react';
import { usePreferences } from './usePreferences.ts';

type ThemeSetting = 'light' | 'dark' | 'system';

/**
 * Reads the user's theme preference and applies the correct data-theme attribute
 * to <html>. For "system", listens to the prefers-color-scheme media query.
 *
 * The CSS variable overrides in globals.css switch palettes based on
 * [data-theme="light"] — dark is the default (no attribute needed).
 */
export function useThemeEffect(): ThemeSetting {
  const { data: prefs } = usePreferences();
  const theme: ThemeSetting = prefs?.ui?.theme ?? 'system';
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const root = document.documentElement;

    function apply(effective: 'light' | 'dark') {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      root.classList.add('transition-colors', 'duration-200');
      if (effective === 'light') {
        root.setAttribute('data-theme', 'light');
      } else {
        root.removeAttribute('data-theme');
      }
      timeoutRef.current = setTimeout(() => {
        root.classList.remove('transition-colors', 'duration-200');
        timeoutRef.current = null;
      }, 300);
    }

    if (theme === 'light') {
      apply('light');
      return () => {
        if (timeoutRef.current) clearTimeout(timeoutRef.current);
      };
    }

    if (theme === 'dark') {
      apply('dark');
      return () => {
        if (timeoutRef.current) clearTimeout(timeoutRef.current);
      };
    }

    // "system" — use media query and listen for changes
    const mq = window.matchMedia('(prefers-color-scheme: light)');
    apply(mq.matches ? 'light' : 'dark');

    function onChange(e: MediaQueryListEvent) {
      apply(e.matches ? 'light' : 'dark');
    }

    mq.addEventListener('change', onChange);
    return () => {
      mq.removeEventListener('change', onChange);
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, [theme]);

  return theme;
}
