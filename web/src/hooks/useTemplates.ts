import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { templatesApi } from '../api/templates.ts';
import type { CreateTemplateParams } from '../types/template.ts';

export function useTemplates(params?: { tag?: string; name?: string }) {
  return useQuery({
    queryKey: ['templates', params],
    queryFn: () => templatesApi.list(params),
  });
}

export function useTemplate(id: string | undefined) {
  return useQuery({
    queryKey: ['templates', id],
    queryFn: () => templatesApi.get(id!),
    enabled: !!id,
  });
}

export function useCreateTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateTemplateParams) => templatesApi.create(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates'] }),
  });
}

export function useUpdateTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: CreateTemplateParams }) =>
      templatesApi.update(id, params),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['templates', id] });
      qc.invalidateQueries({ queryKey: ['templates'] });
    },
  });
}

export function useDeleteTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => templatesApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates'] }),
  });
}

export function useCloneTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => templatesApi.clone(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['templates'] }),
  });
}
