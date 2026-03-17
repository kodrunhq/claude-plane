import { useState, useMemo } from 'react';
import { Link, useNavigate } from 'react-router';
import { Plus, CopyPlus, Trash2, Search, Play } from 'lucide-react';
import { toast } from 'sonner';
import { useTemplates, useDeleteTemplate, useCloneTemplate } from '../hooks/useTemplates.ts';
import { ConfirmDialog } from '../components/shared/ConfirmDialog.tsx';
import { EmptyState } from '../components/shared/EmptyState.tsx';
import { LaunchTemplateModal } from '../components/templates/LaunchTemplateModal.tsx';
import { formatTimeAgo, truncateId } from '../lib/format.ts';
import type { SessionTemplate } from '../types/template.ts';

function formatArgs(args: string[] | undefined): string {
  if (!args || args.length === 0) return '—';
  const joined = args.join(' ');
  return joined.length > 30 ? joined.slice(0, 30) + '...' : joined;
}

function formatWorkingDir(dir: string | undefined): string {
  if (!dir) return '—';
  return dir.length > 25 ? '...' + dir.slice(-22) : dir;
}

export function TemplatesPage() {
  const navigate = useNavigate();
  const { data: templates, isLoading } = useTemplates();
  const deleteTemplate = useDeleteTemplate();
  const cloneTemplate = useCloneTemplate();

  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [launchTemplateId, setLaunchTemplateId] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const filteredTemplates = useMemo(() => {
    const sorted = [...(templates ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at));
    return sorted.filter((t) => {
      return (
        searchQuery === '' ||
        t.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        t.template_id.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (t.description ?? '').toLowerCase().includes(searchQuery.toLowerCase())
      );
    });
  }, [templates, searchQuery]);

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

  async function handleDuplicate(e: React.MouseEvent, template: SessionTemplate) {
    e.stopPropagation();
    try {
      await cloneTemplate.mutateAsync(template.template_id);
      toast.success(`Duplicated "${template.name}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to duplicate template');
    }
  }

  return (
    <div className="p-4 md:p-6 space-y-4">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-xl font-semibold text-text-primary">Templates</h1>
        <Link
          to="/templates/new"
          className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Plus size={16} />
          New Template
        </Link>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-4">
        <div className="relative flex-1 max-w-xs">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
          <input
            type="text"
            placeholder="Search by name, ID, or description..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm pl-9 pr-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/50"
          />
        </div>
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-bg-secondary rounded-lg p-4 animate-pulse">
              <div className="h-4 bg-bg-tertiary rounded w-1/4 mb-2" />
              <div className="h-3 bg-bg-tertiary rounded w-1/2" />
            </div>
          ))}
        </div>
      ) : filteredTemplates.length === 0 ? (
        <EmptyState
          title={templates && templates.length > 0 ? 'No matching templates' : 'No templates yet'}
          description={templates && templates.length > 0 ? 'Try adjusting your search or filters.' : 'Create a template to define reusable session configurations.'}
        />
      ) : (
        <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-text-secondary border-b border-border-primary">
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Command</th>
              <th className="px-4 py-2 hidden md:table-cell">Args</th>
              <th className="px-4 py-2 hidden md:table-cell">Working Dir</th>
              <th className="px-4 py-2 hidden md:table-cell">Updated</th>
              <th className="px-4 py-2"></th>
            </tr>
          </thead>
          <tbody>
            {filteredTemplates.map((template) => (
              <tr
                key={template.template_id}
                onClick={() => navigate(`/templates/${template.template_id}/edit`)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    navigate(`/templates/${template.template_id}/edit`);
                  }
                }}
                tabIndex={0}
                role="button"
                className="bg-bg-secondary hover:bg-bg-tertiary/50 cursor-pointer border-b border-border-primary/50 transition-colors focus:outline-none focus:ring-1 focus:ring-accent-primary"
              >
                <td className="px-4 py-2">
                  <div className="text-text-primary font-medium truncate">{template.name}</div>
                  {template.description && (
                    <div className="text-xs text-text-secondary truncate mt-0.5 max-w-[200px]">{template.description}</div>
                  )}
                  <div className="font-mono text-xs text-text-secondary/60 mt-0.5" title={template.template_id}>
                    {truncateId(template.template_id)}
                  </div>
                </td>
                <td className="px-4 py-2 font-mono text-xs text-text-secondary">
                  {template.command || '—'}
                </td>
                <td className="px-4 py-2 font-mono text-xs text-text-secondary hidden md:table-cell" title={template.args?.join(' ') ?? ''}>
                  {formatArgs(template.args)}
                </td>
                <td className="px-4 py-2 font-mono text-xs text-text-secondary hidden md:table-cell" title={template.working_dir ?? ''}>
                  {formatWorkingDir(template.working_dir)}
                </td>
                <td className="px-4 py-2 text-text-secondary hidden md:table-cell">
                  {formatTimeAgo(template.updated_at)}
                </td>
                <td className="px-4 py-2">
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      onClick={(e) => { e.stopPropagation(); setLaunchTemplateId(template.template_id); }}
                      className="p-1.5 rounded-md text-text-secondary hover:text-accent-primary hover:bg-accent-primary/10 transition-colors"
                      title="Launch"
                    >
                      <Play size={14} />
                    </button>
                    <button
                      onClick={(e) => handleDuplicate(e, template)}
                      className="p-1.5 rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
                      title="Duplicate"
                    >
                      <CopyPlus size={14} />
                    </button>
                    <button
                      onClick={(e) => { e.stopPropagation(); setDeleteId(template.template_id); }}
                      className="p-1.5 rounded-md text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
                      title="Delete"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
      )}

      <ConfirmDialog
        open={deleteId !== null}
        title="Delete Template"
        message="Are you sure you want to delete this template? This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeleteId(null)}
      />

      <LaunchTemplateModal
        open={launchTemplateId !== null}
        onClose={() => setLaunchTemplateId(null)}
        template={templates?.find((t) => t.template_id === launchTemplateId) ?? null}
      />
    </div>
  );
}
