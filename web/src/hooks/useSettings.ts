import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { settingsApi } from '../api/settings.ts';
import type { ServerSettings } from '../api/settings.ts';

export function useServerSettings() {
  return useQuery({ queryKey: ['server-settings'], queryFn: () => settingsApi.get() });
}

export function useUpdateServerSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (settings: Partial<ServerSettings>) => settingsApi.update(settings),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['server-settings'] }),
  });
}
