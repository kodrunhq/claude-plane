import { useTemplates } from '../../hooks/useTemplates.ts';
import type { SessionTemplate } from '../../types/template.ts';

interface TemplatePickerProps {
  onSelect: (template: SessionTemplate) => void;
}

export function TemplatePicker({ onSelect }: TemplatePickerProps) {
  const { data: templates, isLoading } = useTemplates();

  if (isLoading || !templates || templates.length === 0) {
    return null;
  }

  function handleChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const id = e.target.value;
    if (!id) return;
    const found = templates?.find((t) => t.template_id === id);
    if (found) {
      onSelect(found);
    }
    e.target.value = '';
  }

  return (
    <select
      onChange={handleChange}
      defaultValue=""
      className="w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary"
    >
      <option value="">From Template...</option>
      {templates.map((t) => (
        <option key={t.template_id} value={t.template_id}>
          {t.name}
        </option>
      ))}
    </select>
  );
}
