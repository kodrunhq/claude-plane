import { Link } from 'react-router';
import { Pencil } from 'lucide-react';
import type { SessionTemplate } from '../../types/template.ts';

interface TemplateCardProps {
  template: SessionTemplate;
}

export function TemplateCard({ template }: TemplateCardProps) {
  return (
    <Link
      to={`/templates/${template.template_id}/edit`}
      className="bg-bg-secondary border border-border-primary rounded-lg p-4 flex flex-col gap-2 hover:border-accent-primary/30 transition-colors group"
    >
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
        <span className="p-1 rounded-md text-text-secondary/0 group-hover:text-text-secondary transition-colors shrink-0">
          <Pencil size={12} />
        </span>
      </div>

      <div className="text-xs space-y-1 mt-1">
        {template.command && (
          <div className="flex items-baseline gap-2">
            <span className="text-text-secondary/60 shrink-0">cmd</span>
            <span className="font-mono text-text-secondary truncate">
              {template.command}{template.args?.length ? ' ' + template.args.join(' ') : ''}
            </span>
          </div>
        )}
        {template.working_dir && (
          <div className="flex items-baseline gap-2">
            <span className="text-text-secondary/60 shrink-0">dir</span>
            <span className="font-mono text-text-secondary truncate">{template.working_dir}</span>
          </div>
        )}
        {template.initial_prompt && (
          <div className="flex items-baseline gap-2">
            <span className="text-text-secondary/60 shrink-0">prompt</span>
            <span className="text-text-secondary truncate">{template.initial_prompt}</span>
          </div>
        )}
      </div>

      {template.tags && template.tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mt-1">
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
    </Link>
  );
}
