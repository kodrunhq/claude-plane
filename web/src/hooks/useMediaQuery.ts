import { useCallback, useSyncExternalStore } from 'react';

const IS_BROWSER = typeof window !== 'undefined';

/**
 * Subscribes to a CSS media query and returns whether it currently matches.
 * Uses useSyncExternalStore for tear-free reads — no effect-based setState.
 * Safe in non-browser contexts (SSR, test runners): returns false.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (callback: () => void) => {
      if (!IS_BROWSER) return () => {};
      const mql = window.matchMedia(query);
      mql.addEventListener('change', callback);
      return () => mql.removeEventListener('change', callback);
    },
    [query],
  );

  const getSnapshot = useCallback(
    () => IS_BROWSER && window.matchMedia(query).matches,
    [query],
  );
  const getServerSnapshot = useCallback(() => false, []);

  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

/** True when viewport is < 768px (Tailwind `md` breakpoint). */
export function useIsMobile(): boolean {
  return useMediaQuery('(max-width: 767px)');
}
