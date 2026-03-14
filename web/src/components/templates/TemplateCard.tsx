import { Link } from 'react-router';
import { Play, Pencil } from 'lucide-react';
import type { SessionTemplate } from '../../types/template.ts';

interface TemplateCardProps {
  template: SessionTemplate;
  onLaunch: (template: SessionTemplate) => void;
}

export function TemplateCard({ template, onLaunch }: TemplateCardProps) {
  return (
    <div className="bg-bg-secondary border border-border-primary rounded-lg p-4 flex flex-col gap-3 hover:border-accent-primary/30 transition-colors">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold text-text-primary truncate">
            {template.name}
          </h3>
          {template.description && (
            <p className="text-xs text-text-secondary mt-1 line-clamp-2">
              {template.description}
            </p>
          )}
        </div>
      </div>

      {template.tags && template.tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {template.tags.map((tag) => (
            <span
              key={tag}
              className="bg-bg-tertiary text-text-secondary rounded-full px-2 py-0.5 text-xs"
            >
              {tag}
            </span>
          ))}
        </div>
      )}

      <div className="flex items-center gap-2 mt-auto pt-1">
        <button
          onClick={() => onLaunch(template)}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          <Play size={12} />
          Launch
        </button>
        <Link
          to={`/templates/${template.template_id}/edit`}
          className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
        >
          <Pencil size={12} />
          Edit
        </Link>
      </div>
    </div>
  );
}
