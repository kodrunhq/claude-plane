import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { preferencesApi } from '../api/preferences.ts';
import type { UserPreferences } from '../types/preferences.ts';

export function usePreferences() {
  return useQuery({
    queryKey: ['preferences'],
    queryFn: preferencesApi.get,
  });
}

export function useUpdatePreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (prefs: UserPreferences) => preferencesApi.update(prefs),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['preferences'] }),
  });
}

