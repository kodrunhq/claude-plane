import { useState, useMemo } from 'react';
import { Link } from 'react-router';
import { Plus, Pencil, Copy, Trash2, Play } from 'lucide-react';
import { toast } from 'sonner';
import { useTemplates, useDeleteTemplate, useCloneTemplate } from '../hooks/useTemplates.ts';
import { LaunchTemplateModal } from '../components/templates/LaunchTemplateModal.tsx';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { TimeAgo } from '../components/shared/TimeAgo.tsx';
import type { SessionTemplate } from '../types/template.ts';

export function TemplatesPage() {
  const { data: templates, isLoading } = useTemplates();
  const deleteTemplate = useDeleteTemplate();
  const cloneTemplate = useCloneTemplate();

  const [launchTemplate, setLaunchTemplate] = useState<SessionTemplate | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const sortedTemplates = useMemo(
    () => [...(templates ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at)),
    [templates],
  );

  async function handleDelete() {
    if (!deleteId) return;
    try {
      await deleteTemplate.mutateAsync(deleteId);
      toast.success('Template deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete template');
    }
    setDeleteId(null);
  }

  async function handleClone(template: SessionTemplate) {
    try {
      await cloneTemplate.mutateAsync(template.template_id);
      toast.success(`Cloned "${template.name}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to clone template');
    }
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Templates</h1>
        <Link
          to="/templates/new"
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-lg font-medium bg-accent-primary hover:bg-accent-primary/90 text-white transition-all hover:shadow-[0_0_20px_rgba(59,130,246,0.3)]"
        >
          <Plus size={16} />
          New Template
        </Link>
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-bg-secondary rounded-lg border border-border-primary p-4 animate-pulse">
              <div className="h-4 bg-bg-tertiary rounded w-1/4 mb-2" />
              <div className="h-3 bg-bg-tertiary rounded w-1/2" />
            </div>
          ))}
        </div>
      ) : sortedTemplates.length === 0 ? (
        <div className="bg-bg-secondary rounded-lg border border-border-primary p-8 text-center">
          <p className="text-sm text-text-secondary mb-3">
            No templates yet. Create one to define reusable session configurations.
          </p>
          <Link
            to="/templates/new"
            className="inline-flex items-center gap-1.5 text-sm text-accent-primary hover:text-accent-primary/80 transition-colors"
          >
            <Plus size={14} />
            Create your first template
          </Link>
        </div>
      ) : (
        <div className="bg-bg-secondary rounded-lg border border-border-primary divide-y divide-border-primary">
          {sortedTemplates.map((template) => (
            <div
              key={template.template_id}
              className="flex items-center justify-between px-4 py-3 hover:bg-accent-primary/5 transition-all first:rounded-t-lg last:rounded-b-lg"
            >
              <div className="flex-1 min-w-0 mr-4">
                <div className="flex items-center gap-2">
                  <p className="text-sm font-medium text-text-primary truncate">
                    {template.name}
                  </p>
                  {template.tags && template.tags.length > 0 && (
                    <div className="flex gap-1 shrink-0">
                      {template.tags.slice(0, 3).map((tag) => (
                        <span
                          key={tag}
                          className="bg-bg-tertiary text-text-secondary rounded-full px-2 py-0.5 text-xs"
                        >
                          {tag}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
                <p className="text-xs text-text-secondary mt-0.5">
                  {template.description && (
                    <span className="mr-2">{template.description}</span>
                  )}
                  {template.command && (
                    <>
                      <span className="font-mono text-text-secondary/70">{template.command}</span>
                      {' · '}
                    </>
                  )}
                  {template.timeout_seconds > 0 && (
                    <>
                      <span>{template.timeout_seconds}s timeout</span>
                      {' · '}
                    </>
                  )}
                  <TimeAgo date={template.created_at} />
                </p>
              </div>

              <div className="flex items-center gap-1 shrink-0">
                <button
                  onClick={() => setLaunchTemplate(template)}
                  className="p-1.5 rounded-md text-text-secondary hover:text-accent-primary hover:bg-accent-primary/10 transition-colors"
                  title="Launch"
                >
                  <Play size={14} />
                </button>
                <Link
                  to={`/templates/${template.template_id}/edit`}
                  className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
                  title="Edit"
                >
                  <Pencil size={14} />
                </Link>
                <button
                  onClick={() => handleClone(template)}
                  className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
                  title="Clone"
                >
                  <Copy size={14} />
                </button>
                <button
                  onClick={() => setDeleteId(template.template_id)}
                  className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
                  title="Delete"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <LaunchTemplateModal
        open={launchTemplate !== null}
        onClose={() => setLaunchTemplate(null)}
        template={launchTemplate}
      />

      <ConfirmDialog
        open={deleteId !== null}
        title="Delete Template"
        message="Are you sure you want to delete this template? This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeleteId(null)}
      />
    </div>
  );
}
