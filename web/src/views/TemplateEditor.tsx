import { useNavigate, useParams } from 'react-router';
import { toast } from 'sonner';
import { ArrowLeft } from 'lucide-react';
import { TemplateForm } from '../components/templates/TemplateForm.tsx';
import { useTemplate, useCreateTemplate, useUpdateTemplate } from '../hooks/useTemplates.ts';
import type { CreateTemplateParams } from '../types/template.ts';

export function TemplateEditor() {
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const isEditing = !!id;

  const { data: template, isLoading: templateLoading } = useTemplate(id);
  const createTemplate = useCreateTemplate();
  const updateTemplate = useUpdateTemplate();

  async function handleSubmit(params: CreateTemplateParams) {
    try {
      if (isEditing && id) {
        await updateTemplate.mutateAsync({ id, params });
        toast.success('Template updated');
      } else {
        await createTemplate.mutateAsync(params);
        toast.success('Template created');
      }
      navigate('/templates');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save template';
      toast.error(message);
    }
  }

  function handleCancel() {
    navigate('/templates');
  }

  if (isEditing && templateLoading) {
    return (
      <div className="p-4 md:p-6">
        <div className="animate-pulse space-y-4 max-w-2xl">
          <div className="h-6 bg-bg-tertiary rounded w-1/4" />
          <div className="h-10 bg-bg-tertiary rounded" />
          <div className="h-10 bg-bg-tertiary rounded" />
          <div className="h-10 bg-bg-tertiary rounded" />
        </div>
      </div>
    );
  }

  if (isEditing && !template && !templateLoading) {
    return (
      <div className="p-4 md:p-6">
        <p className="text-text-secondary">Template not found.</p>
      </div>
    );
  }

  return (
    <div className="p-4 md:p-6">
      <button
        onClick={handleCancel}
        className="flex items-center gap-1.5 text-sm text-text-secondary hover:text-text-primary mb-4 transition-colors"
      >
        <ArrowLeft size={16} />
        Back to Templates
      </button>

      <h1 className="text-xl font-semibold text-text-primary mb-6">
        {isEditing ? 'Edit Template' : 'New Template'}
      </h1>

      <TemplateForm
        initialValues={template ?? undefined}
        onSubmit={handleSubmit}
        onCancel={handleCancel}
        isLoading={createTemplate.isPending || updateTemplate.isPending}
      />
    </div>
  );
}
